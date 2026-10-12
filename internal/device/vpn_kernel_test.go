package device

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVPNKernelEvidenceFromFixtures(t *testing.T) {
	root := t.TempDir()
	makeFile := func(path, data string) {
		p := filepath.Join(root, path)
		os.MkdirAll(filepath.Dir(p), 0700)
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	makeFile("proc/misc", "  1 psaux\n200 tun\n")
	makeFile("proc/net/xfrm_stat", "XfrmInError 0\n")
	path := filepath.Join(root, "proc/config.gz")
	os.MkdirAll(filepath.Dir(path), 0700)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	_, _ = z.Write([]byte("CONFIG_TUN=y\n# CONFIG_WIREGUARD is not set\n"))
	z.Close()
	f.Close()
	p := InspectVPNKernel(root, true, false, false)
	if p.TUNRegistered == nil || !*p.TUNRegistered || p.TUNKernelConfig != "built-in" || p.WireGuardKernelConfig != "disabled" || !p.XFRMStatPresent {
		t.Fatalf("kernel capabilities parsed incorrectly: %+v", p)
	}
	if !strings.Contains(p.TunAssessment, "registration observed") || !strings.Contains(p.WireGuardAssessment, "userspace alternative") {
		t.Fatal("unsupported assessment", p)
	}
	none := InspectVPNKernel(t.TempDir(), false, false, false)
	if none.TUNRegistered != nil || none.TUNKernelConfig != "unknown" || !strings.Contains(none.TunAssessment, "not demonstrated") {
		t.Fatalf("unknown mistaken for unavailable %+v", none)
	}
}
