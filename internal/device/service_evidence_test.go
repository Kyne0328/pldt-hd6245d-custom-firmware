package device

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDiscoverServiceEvidenceOnlyFixedMetadata(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("etc/resolv.conf", "nameserver 10.0.0.1\nsearch private.example\n")
	write("tmp/dnsmasq.leases", "secret-private-lease-body\n")
	write("var/udhcpd/udhcpd.leases", "private-vendor-lease-data\n")
	write("fhconf/udhcpd.conf", "never-share-configuration\n")
	write("proc/100/comm", "dnsmasq\n")
	write("proc/101/comm", "secret-daemon\n")
	write("proc/net/udp", "  sl  local_address rem_address   st\n   0: 00000000:0043 00000000:0000 07\n   1: 00000000:0035 00000000:0000 07\n")
	write("proc/net/tcp", "  sl  local_address rem_address   st\n   0: 00000000:0035 00000000:0000 0A\n   1: 00000000:0AC0 00000000:0000 01\n")
	ev := DiscoverServiceEvidence(root)
	if !ev.ResolverCandidates[0].Present || ev.ResolverCandidates[0].Kind != "regular file" {
		t.Fatalf("resolver path not observed: %+v", ev.ResolverCandidates)
	}
	var foundLease bool
	for _, p := range ev.LeaseCandidates {
		if p.Label == "/tmp/dnsmasq.leases" && p.Present {
			foundLease = true
		}
	}
	if !foundLease {
		t.Fatal("missing lease path metadata")
	}
	var vendorLease, vendorConfig bool
	for _, p := range ev.LeaseCandidates {
		if p.Label == "/var/udhcpd/udhcpd.leases" && p.Present {
			vendorLease = true
		}
	}
	for _, p := range ev.VendorDHCPConfigCandidates {
		if p.Label == "/fhconf/udhcpd.conf" && p.Present {
			vendorConfig = true
		}
	}
	if !vendorLease || !vendorConfig {
		t.Fatal("firmware-confirmed candidates absent from detection")
	}
	if !slices.Equal(ev.KnownProcessNames, []string{"dnsmasq"}) {
		t.Fatalf("process allowlist not enforced: %+v", ev.KnownProcessNames)
	}
	if !ev.DNSUDP53Observed || !ev.DNSTCP53Listening || !ev.DHCPUDP67Observed {
		t.Fatalf("port numbers not observed: %+v", ev)
	}
	if strings.Contains(strings.Join(ev.KnownProcessNames, " "), "secret") {
		t.Fatal("private process name leaked")
	}
}

func TestServiceDiscoveryAbsentAndSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "private-target")
	if err := os.WriteFile(target, []byte("private contents"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "etc", "resolv.conf")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	ev := DiscoverServiceEvidence(root)
	if !ev.ResolverCandidates[0].Present || ev.ResolverCandidates[0].Kind != "symlink (target not inspected)" {
		t.Fatal("symlink mistaken for file")
	}
	for _, p := range ev.LeaseCandidates {
		if p.Present {
			t.Fatalf("invented lease: %+v", p)
		}
	}
	if ev.DNSUDP53Observed || ev.DNSTCP53Listening || ev.DHCPUDP67Observed {
		t.Fatal("invented socket presence")
	}
	if len(ev.KnownProcessNames) != 0 {
		t.Fatal("invented process names")
	}
}
