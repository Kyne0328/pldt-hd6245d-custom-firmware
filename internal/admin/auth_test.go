package admin

import (
	"encoding/json"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredential(t *testing.T) {
	c, e := NewCredential("long-secure-test-passphrase")
	if e != nil || !c.Verify("long-secure-test-passphrase") || c.Verify("incorrect-password") {
		t.Fatal("credential verification broken", e)
	}
	if _, e := NewCredential("weak"); e == nil {
		t.Fatal("short password accepted")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "admin.json")
	if e = SaveCredential(p, c); e != nil {
		t.Fatal(e)
	}
	if e = SaveCredential(p, c); e == nil {
		t.Fatal("existing credential overwritten")
	}
	loaded, e := LoadCredential(p)
	if e != nil || !loaded.Verify("long-secure-test-passphrase") {
		t.Fatal("reload failed", e)
	}
	if e = os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadCredential(p); e == nil {
		t.Fatal("insecure file permissions accepted")
	}
}
func TestAuthRoutesAndCSRF(t *testing.T) {
	c, _ := NewCredential("long-secure-test-passphrase")
	b := New(c, services.New())
	b.Snapshot = func() device.Status {
		return device.Status{Model: "BCM96846", Interfaces: []device.Interface{}, Disks: []device.Disk{}}
	}
	s := httptest.NewServer(b.Handler())
	defer s.Close()
	client := s.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	req, _ := http.NewRequest("GET", s.URL+"/api/status", nil)
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("API is public", res.StatusCode)
	}
	form := url.Values{"password": {"long-secure-test-passphrase"}}
	req, _ = http.NewRequest("POST", s.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.URL)
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 303 {
		t.Fatal("login not accepted", res.StatusCode)
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("missing HttpOnly session cookie")
	}
	req, _ = http.NewRequest("GET", s.URL+"/api/status", nil)
	req.AddCookie(cookies[0])
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var data map[string]interface{}
	if e = json.NewDecoder(res.Body).Decode(&data); e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 || data["mode"] != "observation-only" {
		t.Fatal("auth API failed", res.StatusCode, data)
	}
	req, _ = http.NewRequest("POST", s.URL+"/logout", nil)
	req.Header.Set("Origin", s.URL)
	req.AddCookie(cookies[0])
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("CSRF protection bypassed")
	}
	req, _ = http.NewRequest("GET", s.URL+"/api/csrf", nil)
	req.AddCookie(cookies[0])
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	var csrf map[string]string
	if e = json.NewDecoder(res.Body).Decode(&csrf); e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	req, _ = http.NewRequest("POST", s.URL+"/logout", nil)
	req.Header.Set("Origin", s.URL)
	req.Header.Set("X-CSRF-Token", csrf["csrf"])
	req.AddCookie(cookies[0])
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal("logout failed", res.StatusCode)
	}
	req, _ = http.NewRequest("GET", s.URL+"/api/status", nil)
	req.AddCookie(cookies[0])
	res, e = client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("session not revoked")
	}
}
func TestNullOriginIsAcceptedOnlyForHTTPSBrowserSameOrigin(t *testing.T) {
	r := httptest.NewRequest("POST", "https://192.168.1.2:9443/login", nil)
	r.Header.Set("Origin", "null")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if !sameOrigin(r) {
		t.Fatal("self-signed HTTPS form incorrectly rejected")
	}
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	if sameOrigin(r) {
		t.Fatal("cross-site null origin accepted")
	}
	r.Header.Del("Sec-Fetch-Site")
	if sameOrigin(r) {
		t.Fatal("unattributed null origin accepted")
	}
	plain := httptest.NewRequest("POST", "http://192.168.1.2:9443/login", nil)
	plain.Header.Set("Origin", "null")
	plain.Header.Set("Sec-Fetch-Site", "same-origin")
	if sameOrigin(plain) {
		t.Fatal("insecure null origin accepted")
	}
}

func TestBindSecurity(t *testing.T) {
	for _, a := range []string{"0.0.0.0:9080", ":9080", "192.168.1.2:9080"} {
		if SecureListen(a, "", "") == nil {
			t.Fatal("insecure external bind allowed", a)
		}
	}
	if e := SecureListen("127.0.0.1:9080", "", ""); e != nil {
		t.Fatal(e)
	}
	if e := SecureListen("0.0.0.0:9080", "server.crt", "server.key"); e != nil {
		t.Fatal(e)
	}
}
