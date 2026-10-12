package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/services"
)

const rounds = 300000

type Credential struct {
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Iterations int    `json:"iterations"`
}

func derive(password, salt []byte, n int) []byte {
	// PBKDF2-HMAC-SHA256, single 32-byte output block.
	var block = [4]byte{0, 0, 0, 1}
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write(block[:])
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < n; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
func NewCredential(password string) (Credential, error) {
	if len(password) < 12 {
		return Credential{}, errors.New("admin password must be at least 12 characters")
	}
	salt := make([]byte, 24)
	if _, e := rand.Read(salt); e != nil {
		return Credential{}, e
	}
	return Credential{Salt: base64.RawStdEncoding.EncodeToString(salt), Hash: base64.RawStdEncoding.EncodeToString(derive([]byte(password), salt, rounds)), Iterations: rounds}, nil
}
func (c Credential) Verify(password string) bool {
	salt, e1 := base64.RawStdEncoding.DecodeString(c.Salt)
	expected, e2 := base64.RawStdEncoding.DecodeString(c.Hash)
	if e1 != nil || e2 != nil || len(salt) < 16 || len(expected) != 32 || c.Iterations < 100000 || c.Iterations > 600000 {
		return false
	}
	actual := derive([]byte(password), salt, c.Iterations)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
func SaveCredential(path string, c Credential) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	e = json.NewEncoder(f).Encode(c)
	if e != nil {
		_ = os.Remove(path)
	}
	return e
}
func LoadCredential(path string) (Credential, error) {
	st, e := os.Stat(path)
	if e != nil {
		return Credential{}, e
	}
	if st.Mode().Perm()&0077 != 0 {
		return Credential{}, errors.New("credential file permissions must be 0600")
	}
	f, e := os.Open(path)
	if e != nil {
		return Credential{}, e
	}
	defer f.Close()
	var c Credential
	e = json.NewDecoder(f).Decode(&c)
	return c, e
}

type session struct {
	expires time.Time
	csrf    string
}
type Backend struct {
	Credential Credential
	Supervisor *services.Supervisor
	Snapshot   func() device.Status
	mu         sync.Mutex
	sessions   map[string]session
	failures   map[string][]time.Time
}

func New(c Credential, supervisor *services.Supervisor) *Backend {
	if supervisor == nil {
		supervisor = services.New()
	}
	return &Backend{Credential: c, Supervisor: supervisor, Snapshot: device.Snapshot, sessions: map[string]session{}, failures: map[string][]time.Time{}}
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (b *Backend) authenticated(r *http.Request) (session, bool) {
	c, e := r.Cookie("hgfw_session")
	if e != nil {
		return session{}, false
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.Value)))
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.sessions[hash]
	if !ok {
		return session{}, false
	}
	if time.Now().After(s.expires) {
		delete(b.sessions, hash)
		return session{}, false
	}
	return s, true
}
func (b *Backend) rateLimit(ip string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	old := b.failures[ip]
	recent := []time.Time{}
	for _, t := range old {
		if now.Sub(t) < 5*time.Minute {
			recent = append(recent, t)
		}
	}
	b.failures[ip] = recent
	return len(recent) < 5
}
func (b *Backend) failed(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures[ip] = append(b.failures[ip], time.Now())
}
func (client *Backend) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := client.authenticated(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(loginHTML)
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "" {
			host = r.RemoteAddr
		}
		// Browsers submit same-origin only; do not accept cross-origin POSTs.
		if !sameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}
		if !client.rateLimit(host) {
			http.Error(w, "Too many attempts; retry later", http.StatusTooManyRequests)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if e := r.ParseForm(); e != nil {
			http.Error(w, "Malformed login", http.StatusBadRequest)
			return
		}
		if !client.Credential.Verify(r.PostForm.Get("password")) {
			client.failed(host)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		token, e := randomToken()
		if e != nil {
			http.Error(w, "Authentication unavailable", 500)
			return
		}
		csrf, e := randomToken()
		if e != nil {
			http.Error(w, "Authentication unavailable", 500)
			return
		}
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
		client.mu.Lock()
		if len(client.sessions) > 128 {
			client.sessions = map[string]session{}
		}
		client.sessions[h] = session{expires: time.Now().Add(30 * time.Minute), csrf: csrf}
		delete(client.failures, host)
		client.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "hgfw_session", Value: token, Path: "/", MaxAge: 1800, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if _, ok := client.authenticated(r); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := client.authenticated(r); !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(struct {
			Device   device.Status    `json:"device"`
			Services []services.State `json:"services"`
			Mode     string           `json:"mode"`
		}{client.Snapshot(), client.Supervisor.Snapshot(), "observation-only"})
	})
	mux.HandleFunc("GET /api/csrf", func(w http.ResponseWriter, r *http.Request) {
		s, ok := client.authenticated(r)
		if !ok {
			http.Error(w, "Unauthorized", 401)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"csrf": s.csrf})
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "Invalid request origin", 403)
			return
		}
		s, ok := client.authenticated(r)
		if !ok {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if subtle.ConstantTimeCompare([]byte(s.csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
			http.Error(w, "Invalid CSRF token", 403)
			return
		}
		cookie, _ := r.Cookie("hgfw_session")
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(cookie.Value)))
		client.mu.Lock()
		delete(client.sessions, h)
		client.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "hgfw_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /assets/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(styleCSS)
	})
	mux.HandleFunc("GET /assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(appJS)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		mux.ServeHTTP(w, r)
	})
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	return origin == "http://"+r.Host && r.TLS == nil || origin == "https://"+r.Host && r.TLS != nil
}
func SecureListen(addr, cert, key string) error {
	host, _, e := net.SplitHostPort(addr)
	if e != nil {
		return e
	}
	ip := net.ParseIP(host)
	local := ip != nil && ip.IsLoopback() || host == "localhost"
	if !local && (cert == "" || key == "") {
		return errors.New("external dashboard binds require explicit TLS cert and key")
	}
	if (cert == "") != (key == "") {
		return errors.New("TLS requires certificate and key")
	}
	return nil
}
func ParseAddr(addr string) (int, error) {
	_, port, e := net.SplitHostPort(addr)
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(port)
}
