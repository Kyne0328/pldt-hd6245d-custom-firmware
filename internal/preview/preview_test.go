package preview

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRejectUnsafeBind(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:9443", ":9443", "192.0.2.10:9443", "[::]:9443", "192.168.1.2:99999", "bad-host:9443"} {
		if _, err := LocalIPv4Listen(addr); err == nil {
			t.Fatalf("accepted unsafe bind %s", addr)
		}
	}
	if _, err := LocalIPv4Listen("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
}
func TestPreviewTLSAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readyCh := make(chan Ready, 1)
	done := make(chan error, 1)
	go func() {
		done <- RunContext(ctx, Options{
			Listen:   "127.0.0.1:0",
			Duration: 30 * time.Second,
			OnReady:  func(v Ready) { readyCh <- v },
		})
	}()
	var ready Ready
	select {
	case ready = <-readyCh:
	case e := <-done:
		t.Fatalf("preview failed: %v", e)
	case <-time.After(15 * time.Second):
		t.Fatal("preview did not start")
	}
	sum := sha256.Sum256(ready.Certificate)
	if hex.EncodeToString(sum[:]) != ready.Fingerprint || len(ready.Password) != 32 {
		t.Fatal("bad temporary secret or fingerprint")
	}
	cert, e := x509.ParseCertificate(ready.Certificate)
	if e != nil {
		t.Fatal(e)
	}
	if len(cert.IPAddresses) != 1 || !cert.IPAddresses[0].IsLoopback() {
		t.Fatal("cert SAN not loopback")
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	client := &http.Client{
		Timeout:       12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
	get := func(path string, cookie *http.Cookie) *http.Response {
		req, _ := http.NewRequest("GET", ready.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		r, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	post := func(path, body, csrf string, cookie *http.Cookie) *http.Response {
		req, _ := http.NewRequest("POST", ready.URL+path, strings.NewReader(body))
		req.Header.Set("Origin", "null")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		r, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	check := func(r *http.Response, code int) {
		defer r.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 2<<20))
		if r.StatusCode != code {
			t.Fatalf("got HTTP %d, want %d", r.StatusCode, code)
		}
	}
	check(get("/api/status", nil), 401)
	check(get("/login", nil), 200)
	r := post("/login", url.Values{"password": {ready.Password}}.Encode(), "", nil)
	if r.StatusCode != 303 {
		t.Fatal("login denied", r.StatusCode)
	}
	var cookie *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == "hgfw_session" {
			cookie = c
		}
	}
	_ = r.Body.Close()
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("login cookie protection missing")
	}
	check(get("/", cookie), 200)
	check(get("/assets/style.css", cookie), 200)
	r = get("/api/status", cookie)
	var api struct {
		Mode string `json:"mode"`
	}
	decErr := json.NewDecoder(r.Body).Decode(&api)
	r.Body.Close()
	if decErr != nil || r.StatusCode != 200 || api.Mode != "observation-only" {
		t.Fatal("dashboard API failed", api, r.StatusCode, decErr)
	}
	check(post("/logout", "", "", cookie), 403)
	r = get("/api/csrf", cookie)
	var csrf struct {
		CSRF string `json:"csrf"`
	}
	decErr = json.NewDecoder(r.Body).Decode(&csrf)
	r.Body.Close()
	if decErr != nil || csrf.CSRF == "" {
		t.Fatal("missing CSRF token")
	}
	check(post("/logout", "", csrf.CSRF, cookie), 204)
	check(get("/api/status", cookie), 401)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server failed to stop after context cancelled")
	}
	// Make sure the same ephemeral TCP port is no longer listening.
	_, err := client.Get(ready.URL + "/login")
	if err == nil {
		t.Fatal("HTTP server still reachable after shutdown")
	}
}
func TestPreviewTimeoutLimits(t *testing.T) {
	if err := Run(Options{Listen: "127.0.0.1:0", Duration: time.Second}); err == nil {
		t.Fatal("short TTL accepted")
	}
	if err := Run(Options{Listen: "127.0.0.1:0", Duration: 11 * time.Minute}); err == nil {
		t.Fatal("excessive TTL accepted")
	}
	if _, err := LocalIPv4Listen("127.0.0.1:9443"); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
