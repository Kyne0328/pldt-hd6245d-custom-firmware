// Package netprobe tests isolated loopback-only networking under the
// existing HGFW process supervisor. It never creates interfaces or routes.
package netprobe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
)

const response = "HGFW_LOOPBACK_OK\n"

type Result struct {
	Success          bool   `json:"success"`
	Listen           string `json:"listen"`
	ResponseVerified bool   `json:"responseVerified"`
	Supervised       bool   `json:"supervised"`
	Stopped          bool   `json:"stopped"`
	ElapsedMillis    int64  `json:"elapsedMillis"`
}

func validLoopback(addr string) bool {
	host, port, e := net.SplitHostPort(addr)
	if e != nil || host != "127.0.0.1" {
		return false
	}
	n, e := strconv.Atoi(port)
	return e == nil && n >= 1 && n <= 65535
}

// Child starts a bounded TCP HTTP health responder, listening only on IPv4
// loopback, then stops on SIGTERM or a hard 25-second deadline.
func Child(addr string) error {
	if !validLoopback(addr) {
		return errors.New("probe child requires explicit 127.0.0.1:<port>")
	}
	l, e := net.Listen("tcp4", addr)
	if e != nil {
		return e
	}
	defer l.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, response)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(l) }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	case e = <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return e
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return server.Shutdown(shutdown)
}

// SelfTest starts a separate child process through the real process supervisor,
// verifies HTTP health from the same host, and waits for process-group shutdown.
// It does not create a network namespace; "isolated" here means loopback-bound,
// temporary and independent of the router's existing network configuration.
func SelfTest() (Result, error) {
	start := time.Now()
	result := Result{Listen: "127.0.0.1:ephemeral"}
	exe, e := os.Executable()
	if e != nil {
		return result, e
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return result, e
	}
	addr := l.Addr().String()
	l.Close()
	if !validLoopback(addr) {
		return result, errors.New("unexpected bind address")
	}
	result.Listen = addr
	sup := services.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sup.Run(ctx, services.Config{Services: []services.Definition{{
			Name: "loopback-health-probe", Exec: exe, Args: []string{"network-probe-child", addr},
			Enabled: true, Restart: false,
		}}})
		close(done)
	}()
	defer cancel()
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	deadline := time.Now().Add(14 * time.Second)
	for time.Now().Before(deadline) {
		st := sup.Snapshot()
		if len(st) > 0 && st[0].State == "failed" {
			break
		}
		req, err := http.NewRequest("GET", "http://"+addr+"/health", nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
				resp.Body.Close()
				if resp.StatusCode == 200 && strings.TrimSpace(string(b)) == strings.TrimSpace(response) {
					result.ResponseVerified = true
					// A healthy child can answer before the independent supervisor's
					// state snapshot transitions from starting to running. Do not
					// stop polling before both facts are observed together.
					current := sup.Snapshot()
					result.Supervised = len(current) > 0 && current[0].State == "running"
					if result.Supervised {
						break
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		return result, errors.New("supervisor did not terminate child")
	}
	st := sup.Snapshot()
	result.Stopped = len(st) > 0 && st[0].State == "stopped"
	result.ElapsedMillis = time.Since(start).Milliseconds()
	result.Success = result.ResponseVerified && result.Supervised && result.Stopped
	if !result.Success {
		return result, fmt.Errorf("loopback probe failed: response=%v supervised=%v stopped=%v states=%v", result.ResponseVerified, result.Supervised, result.Stopped, st)
	}
	return result, nil
}
