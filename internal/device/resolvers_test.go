package device

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectResolversBoundedNoDomains(t *testing.T) {
	p := filepath.Join(t.TempDir(), "resolv.conf")
	body := "# private data\nsearch internal.example\nnameserver 192.0.2.53\nnameserver 192.0.2.53\nnameserver invalid\nnameserver 0.0.0.0\nnameserver 2001:db8::53\nnameserver 224.0.0.1\n"
	if e := os.WriteFile(p, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	got := InspectResolvers(p)
	if !got.Available || len(got.Nameservers) != 2 || got.Nameservers[0] != "192.0.2.53" || got.Nameservers[1] != "2001:db8::53" {
		t.Fatalf("bad resolvers %+v", got)
	}
	if strings.Contains(strings.Join(got.Nameservers, " "), "internal.example") {
		t.Fatal("local search domain exposed")
	}
	absent := InspectResolvers(p + ".missing")
	if absent.Available || len(absent.Nameservers) != 0 {
		t.Fatalf("invented resolver: %+v", absent)
	}
}
