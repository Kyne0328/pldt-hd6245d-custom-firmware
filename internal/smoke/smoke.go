// Package smoke runs the actual HGFW HTTP handler on a one-time 127.0.0.1
// listener and verifies its authenticated routes. It uses in-memory random
// credentials, does not create files, and cannot expose a network listener.
package smoke

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/admin"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
)

type Result struct {
	Success        bool   `json:"success"`
	Listener       string `json:"listener"`
	UnauthedDenied bool   `json:"unauthenticatedDenied"`
	LoginSucceeded bool   `json:"loginSucceeded"`
	DashboardOK    bool   `json:"dashboardOK"`
	DeviceAPI      bool   `json:"authenticatedDeviceAPI"`
	CSRFEnforced   bool   `json:"csrfEnforced"`
	LogoutRevoked  bool   `json:"logoutRevoked"`
	ShutdownOK     bool   `json:"shutdownOK"`
	DeviceModel    string `json:"deviceModel"`
	Kernel         string `json:"kernel"`
	Interfaces     int    `json:"interfaceCount"`
	USBDevices     int    `json:"usbBlockDevices"`
	MemoryRSSKiB   int64  `json:"rssKiB"`
	PeakRSSKiB     int64  `json:"peakRSSKiB"`
	MemoryAvailKiB int64  `json:"systemMemAvailableKiB"`
	ElapsedMillis  int64  `json:"elapsedMillis"`
}

// Run is safe to execute as root or as a normal user: it listens on loopback
// only, with OS-chosen ephemeral port. It runs no supervision, mounts, writes,
// network-configuration changes, shell commands, or background service.
func Run() (Result, error) {
	start := time.Now()
	result := Result{Listener: "127.0.0.1:ephemeral"}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return result, err
	}
	password := base64.RawURLEncoding.EncodeToString(secret)
	cred, err := admin.NewCredential(password)
	if err != nil {
		return result, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return result, fmt.Errorf("loopback bind failed: %w", err)
	}
	defer listener.Close()
	if tcp, ok := listener.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		return result, errors.New("listener is not loopback")
	}
	srv := &http.Server{
		Handler:           admin.New(cred, services.New()).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       15 * time.Second,
		MaxHeaderBytes:    8192,
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	base := "http://" + listener.Addr().String()
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
	}
	defer client.CloseIdleConnections()
	get := func(path string, cookie *http.Cookie) (*http.Response, error) {
		req, e := http.NewRequest("GET", base+path, nil)
		if e != nil {
			return nil, e
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		return client.Do(req)
	}
	post := func(path, body string, cookie *http.Cookie, csrf string) (*http.Response, error) {
		req, e := http.NewRequest("POST", base+path, strings.NewReader(body))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Origin", base)
		if path == "/login" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		return client.Do(req)
	}
	check := func(response *http.Response, err error, expected int) error {
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2<<20))
		if response.StatusCode != expected {
			return fmt.Errorf("HTTP status %d instead of %d", response.StatusCode, expected)
		}
		return nil
	}
	r, err := get("/api/status", nil)
	if err = check(r, err, http.StatusUnauthorized); err != nil {
		return result, fmt.Errorf("anonymous API restriction: %w", err)
	}
	result.UnauthedDenied = true

	r, err = get("/login", nil)
	if err = check(r, err, http.StatusOK); err != nil {
		return result, fmt.Errorf("login page: %w", err)
	}

	r, err = post("/login", url.Values{"password": {password}}.Encode(), nil, "")
	if err != nil {
		return result, fmt.Errorf("login request: %w", err)
	}
	cookies := r.Cookies()
	if err = check(r, nil, http.StatusSeeOther); err != nil {
		return result, fmt.Errorf("login status: %w", err)
	}
	var cookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "hgfw_session" && c.HttpOnly && c.SameSite == http.SameSiteStrictMode {
			cookie = c
		}
	}
	if cookie == nil {
		return result, errors.New("no secure session cookie")
	}
	result.LoginSucceeded = true

	r, err = get("/", cookie)
	if err = check(r, err, http.StatusOK); err != nil {
		return result, fmt.Errorf("authenticated dashboard: %w", err)
	}
	result.DashboardOK = true

	r, err = get("/api/status", cookie)
	if err != nil {
		return result, fmt.Errorf("authenticated API: %w", err)
	}
	var payload struct {
		Device device.Status `json:"device"`
		Mode   string        `json:"mode"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&payload)
	_ = r.Body.Close()
	if r.StatusCode != http.StatusOK || decodeErr != nil || payload.Mode != "observation-only" {
		return result, fmt.Errorf("authenticated API invalid: status=%d parse=%v", r.StatusCode, decodeErr)
	}
	result.DeviceAPI = true
	result.DeviceModel = payload.Device.Model
	result.Kernel = payload.Device.Kernel
	result.Interfaces = len(payload.Device.Interfaces)
	result.USBDevices = len(payload.Device.Disks)
	result.MemoryAvailKiB = payload.Device.MemAvailKB

	r, err = post("/logout", "", cookie, "")
	if err = check(r, err, http.StatusForbidden); err != nil {
		return result, fmt.Errorf("CSRF restriction: %w", err)
	}
	result.CSRFEnforced = true
	r, err = get("/api/csrf", cookie)
	if err != nil {
		return result, fmt.Errorf("CSRF token request: %w", err)
	}
	var token struct {
		CSRF string `json:"csrf"`
	}
	decodeErr = json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&token)
	_ = r.Body.Close()
	if r.StatusCode != http.StatusOK || decodeErr != nil || token.CSRF == "" {
		return result, errors.New("could not retrieve CSRF token")
	}
	r, err = post("/logout", "", cookie, token.CSRF)
	if err = check(r, err, http.StatusNoContent); err != nil {
		return result, fmt.Errorf("logout: %w", err)
	}
	r, err = get("/api/status", cookie)
	if err = check(r, err, http.StatusUnauthorized); err != nil {
		return result, fmt.Errorf("logout did not revoke session: %w", err)
	}
	result.LogoutRevoked = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = srv.Shutdown(ctx); err != nil {
		return result, fmt.Errorf("HTTP shutdown: %w", err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return result, fmt.Errorf("unexpected HTTP exit: %w", err)
		}
	case <-ctx.Done():
		return result, errors.New("server failed to stop")
	}
	result.ShutdownOK = true
	runtime.GC()
	result.MemoryRSSKiB = procStatusKB("VmRSS:")
	result.PeakRSSKiB = procStatusKB("VmHWM:")
	result.ElapsedMillis = time.Since(start).Milliseconds()
	result.Success = true
	return result, nil
}

func procStatusKB(key string) int64 {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, key) {
			fields := strings.Fields(strings.TrimPrefix(line, key))
			if len(fields) > 0 {
				n, _ := strconv.ParseInt(fields[0], 10, 64)
				return n
			}
		}
	}
	return 0
}
