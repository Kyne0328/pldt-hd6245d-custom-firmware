package preview

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/admin"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
)

// Ready contains a one-time password and certificate fingerprint. Only print
// it to the locally authenticated terminal; never log it to a persistent file.
type Ready struct {
	URL         string
	Password    string
	Fingerprint string
	Certificate []byte
}
type Options struct {
	Listen   string
	Duration time.Duration
	OnReady  func(Ready)
}

func LocalIPv4Listen(addr string) (net.IP, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return nil, errors.New("use a specific IPv4 address assigned to this device")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 0 || p > 65535 {
		return nil, errors.New("invalid TCP port")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var addrIP net.IP
			switch x := a.(type) {
			case *net.IPNet:
				addrIP = x.IP
			case *net.IPAddr:
				addrIP = x.IP
			}
			if addrIP != nil && addrIP.Equal(ip) {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("%s is not assigned to this device", host)
}
func selfSigned(ip net.IP) (tls.Certificate, []byte, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, nil, "", err
	}
	cert := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "HGFW temporary device preview"},
		NotBefore:             time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{ip},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, "", err
	}
	c := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}
	sum := sha256.Sum256(der)
	return c, der, hex.EncodeToString(sum[:]), nil
}
func Run(opts Options) error {
	return RunContext(context.Background(), opts)
}
func RunContext(parent context.Context, opts Options) error {
	if opts.Duration < 20*time.Second || opts.Duration > 10*time.Minute {
		return errors.New("preview duration must be between 20 seconds and 10 minutes")
	}
	ip, err := LocalIPv4Listen(opts.Listen)
	if err != nil {
		return err
	}
	// Restrict the bind to this exact interface address, never all addresses.
	l, err := net.Listen("tcp4", opts.Listen)
	if err != nil {
		return err
	}
	defer l.Close()
	passwordBytes := make([]byte, 24)
	if _, err = rand.Read(passwordBytes); err != nil {
		return err
	}
	password := base64.RawURLEncoding.EncodeToString(passwordBytes)
	credential, err := admin.NewCredential(password)
	if err != nil {
		return err
	}
	cert, der, fingerprint, err := selfSigned(ip)
	if err != nil {
		return err
	}
	s := &http.Server{
		Handler:           admin.New(credential, services.New()).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       20 * time.Second,
		MaxHeaderBytes:    8192,
	}
	tlsListener := tls.NewListener(l, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	done := make(chan error, 1)
	go func() { done <- s.Serve(tlsListener) }()
	if opts.OnReady != nil {
		opts.OnReady(Ready{
			URL:         "https://" + l.Addr().String(),
			Password:    password,
			Fingerprint: fingerprint,
			Certificate: der,
		})
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	t := time.NewTimer(opts.Duration)
	defer t.Stop()
	var serveErr error
	select {
	case <-t.C:
	case <-ctx.Done():
	case serveErr = <-done:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("preview server stopped unexpectedly: %w", serveErr)
		}
		return errors.New("preview server closed unexpectedly")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = s.Shutdown(shutdownCtx); err != nil {
		return err
	}
	select {
	case serveErr = <-done:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("preview exited unexpectedly: %w", serveErr)
		}
	case <-shutdownCtx.Done():
		return errors.New("preview did not stop")
	}
	return nil
}
