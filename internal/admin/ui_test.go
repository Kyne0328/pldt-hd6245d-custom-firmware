package admin

import (
	"bytes"
	"testing"
)

func TestEmbeddedDashboardViews(t *testing.T) {
	for _, view := range [][]byte{[]byte("data-view=\"overview\""), []byte("data-view=\"network\""), []byte("data-view=\"storage\""), []byte("data-view=\"services\"")} {
		if !bytes.Contains(indexHTML, view) {
			t.Fatalf("dashboard view missing: %s", view)
		}
	}
	for _, id := range [][]byte{[]byte("id=\"interface-search\""), []byte("id=\"memory-ring\""), []byte("id=\"sync-label\""), []byte("id=\"logout\" aria-label=\"Sign out\"")} {
		if !bytes.Contains(indexHTML, id) {
			t.Fatalf("dashboard control missing: %s", id)
		}
	}
	if !bytes.Contains(appJS, []byte("/api/status")) || !bytes.Contains(appJS, []byte("textContent")) {
		t.Fatal("dashboard must render server status safely")
	}
	if bytes.Contains(appJS, []byte("innerHTML")) {
		t.Fatal("dashboard must not inject untrusted HTML")
	}
	if bytes.Contains(indexHTML, []byte("https://fonts.")) || bytes.Contains(indexHTML, []byte("<script src=\"http")) {
		t.Fatal("dashboard must not require third-party assets")
	}
	if !bytes.Contains(styleCSS, []byte("@media(max-width:700px)")) {
		t.Fatal("mobile layout missing")
	}
	if !bytes.Contains(loginHTML, []byte("Administrator password")) {
		t.Fatal("login form missing")
	}
}
