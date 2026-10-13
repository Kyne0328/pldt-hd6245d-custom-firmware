// Package printing supports locally attached USB printer detection and an
// explicit, bounded raw TCP print server. No printer is started automatically.
// The network service requires operator confirmation and a private LAN subnet.
package printing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Printer struct {
	Device    string `json:"device"`
	Connected bool   `json:"connected"`
	Protocol  string `json:"protocol"`
}

var lpName = regexp.MustCompile(`^lp(0|[1-9][0-9]?)$`)

func List(classDir, devDir string) []Printer {
	result := []Printer{}
	entries, err := os.ReadDir(classDir)
	if err != nil {
		return result
	}
	for _, item := range entries {
		name := item.Name()
		if !lpName.MatchString(name) {
			continue
		}
		path, err := filepath.EvalSymlinks(filepath.Join(classDir, name))
		if err != nil || !strings.Contains(filepath.ToSlash(path), "/usb") {
			continue
		}
		// /sys/class/usb/lpN is kernel usblp class evidence; do not fabricate
		// printers from USB storage device names or generic /dev nodes.
		for _, candidate := range []string{filepath.Join(devDir, "usb", name), filepath.Join(devDir, name)} {
			stat, err := os.Lstat(candidate)
			if err == nil && stat.Mode()&os.ModeDevice != 0 && stat.Mode()&os.ModeCharDevice != 0 {
				result = append(result, Printer{Device: candidate, Connected: true, Protocol: "USB printer class (raw)"})
				break
			}
		}
	}
	return result
}

type Options struct {
	Listen      string
	AllowedCIDR string
	Device      string
	MaxBytes    int64
	MaxJobs     int
	Duration    time.Duration
	// OpenDevice is test-only injection. Never expose it via the CLI.
	OpenDevice func() (io.WriteCloser, error)
}

func ValidateOptions(o Options) (netip.Prefix, netip.Addr, error) {
	allowed, err := netip.ParsePrefix(o.AllowedCIDR)
	if err != nil || !allowed.Addr().Is4() || allowed.Addr().IsUnspecified() {
		return netip.Prefix{}, netip.Addr{}, errors.New("printer requires an explicit IPv4 subnet")
	}
	host, port, err := net.SplitHostPort(o.Listen)
	if err != nil {
		return netip.Prefix{}, netip.Addr{}, err
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.Is4() || address.IsUnspecified() || address.IsMulticast() ||
		!allowed.Contains(address) {
		return netip.Prefix{}, netip.Addr{}, errors.New("printer listen address must be inside allowed IPv4 subnet")
	}
	pn, e := strconv.Atoi(port)
	if e != nil || pn < 1 || pn > 65535 {
		return netip.Prefix{}, netip.Addr{}, errors.New("invalid printer TCP port")
	}
	if o.MaxBytes < 1 || o.MaxBytes > 4*1024*1024 || o.MaxJobs < 1 || o.MaxJobs > 20 ||
		o.Duration < time.Second || o.Duration > 30*time.Minute {
		return netip.Prefix{}, netip.Addr{}, errors.New("invalid print job/time limits")
	}
	if !address.IsLoopback() && (!address.IsPrivate() || !allowed.Addr().IsPrivate()) {
		return netip.Prefix{}, netip.Addr{}, errors.New("public printer server disabled")
	}
	return allowed, address, nil
}

func realPrinter(device string) error {
	candidates := List("/sys/class/usb", "/dev")
	for _, p := range candidates {
		if p.Device == device {
			return nil
		}
	}
	return errors.New("USB printer not verified by kernel; install usblp only after hardware verification")
}

func receive(conn net.Conn, limit int64, writer func() (io.WriteCloser, error)) error {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var data bytes.Buffer
	if _, err := io.Copy(&data, io.LimitReader(conn, limit+1)); err != nil {
		return err
	}
	if data.Len() == 0 {
		return errors.New("empty print job")
	}
	if int64(data.Len()) > limit {
		return errors.New("print job exceeds limit")
	}
	out, err := writer()
	if err != nil {
		return err
	}
	_, writeErr := io.Copy(out, &data)
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// Serve is an optional LAN print bridge, not a spooler or IPP/CUPS driver.
// It reads an entire bounded job before sending bytes to a locally verified
// USB printer. The server stops after MaxJobs or Duration; no restart hooks.
func Serve(ctx context.Context, o Options) error {
	prefix, _, err := ValidateOptions(o)
	if err != nil {
		return err
	}
	if o.OpenDevice == nil {
		if err = realPrinter(o.Device); err != nil {
			return err
		}
		o.OpenDevice = func() (io.WriteCloser, error) { return os.OpenFile(o.Device, os.O_WRONLY, 0) }
	}
	listener, err := net.Listen("tcp4", o.Listen)
	if err != nil {
		return fmt.Errorf("printer bind: %w", err)
	}
	defer listener.Close()
	deadline := time.Now().Add(o.Duration)
	count := 0
	for count < o.MaxJobs {
		if err = ctx.Err(); err != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return nil
		}
		_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
		conn, acceptErr := listener.Accept()
		if ne, ok := acceptErr.(net.Error); ok && ne.Timeout() {
			continue
		}
		if acceptErr != nil {
			return acceptErr
		}
		remote := conn.RemoteAddr().(*net.TCPAddr)
		addr, addrOK := netip.AddrFromSlice(remote.IP)
		if !addrOK || !prefix.Contains(addr.Unmap()) {
			_ = conn.Close()
			continue
		}
		if err = receive(conn, o.MaxBytes, o.OpenDevice); err != nil {
			return err
		}
		count++
	}
	return nil
}
