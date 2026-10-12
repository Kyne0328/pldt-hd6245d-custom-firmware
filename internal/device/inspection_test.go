package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectNetworkEvidenceAndRoutes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"br0", "eth0.0", "wl0", "gpon0", "wan0"} {
		base := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(base, "statistics"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, v string) {
		if err := os.WriteFile(filepath.Join(root, p), []byte(v), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("eth0.0/operstate", "up\n")
	write("eth0.0/carrier", "1\n")
	write("eth0.0/speed", "1000\n")
	write("eth0.0/statistics/rx_bytes", "2048\n")
	write("eth0.0/statistics/tx_bytes", "4096\n")
	if err := os.Mkdir(filepath.Join(root, "br0", "bridge"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "br0"), filepath.Join(root, "eth0.0", "master")); err != nil {
		t.Fatal(err)
	}
	routes := filepath.Join(t.TempDir(), "route")
	data := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0.0\t00000000\t0101A8C0\t0003\t0\t0\t20\t00000000\t0\t0\t0\n" +
		"br0\t0001A8C0\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
		"garbage\n" +
		"wan0\t00000000\t0201A8C0\t0000\t0\t0\t0\t00000000\t0\t0\t0\n"
	if err := os.WriteFile(routes, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	names := []Interface{{Name: "br0"}, {Name: "eth0.0"}, {Name: "wl0"}, {Name: "gpon0"}, {Name: "wan0"}}
	n := InspectNetwork(names, root, routes)
	if len(n.Interfaces) != 5 || len(n.IPv4Routes) != 2 {
		t.Fatalf("inventory %+v", n)
	}
	if n.DefaultInterface != "eth0.0" || n.DefaultGateway != "192.168.1.1" {
		t.Fatalf("incorrect route %+v", n)
	}
	if n.Interfaces[0].Confidence != "kernel-sysfs" {
		t.Fatal("bridge not verified", n.Interfaces[0])
	}
	e := n.Interfaces[1]
	if e.Master != "br0" || e.Carrier == nil || !*e.Carrier || e.SpeedMbps != 1000 || e.RXBytes != 2048 || e.TXBytes != 4096 {
		t.Fatal("sysfs evidence missing", e)
	}
	if n.Interfaces[2].Confidence != "name-only" || n.Interfaces[3].Role != "Optical/vendor interface" || n.Interfaces[4].Confidence != "name-only" {
		t.Fatalf("unsafe role confidence %+v", n.Interfaces)
	}
	if n.Summary() == "" {
		t.Fatal("empty summary")
	}
}
func TestMissingNetworkFilesRemainUnknown(t *testing.T) {
	r := InspectNetwork([]Interface{{Name: "wl1"}, {Name: "eth2.0"}}, filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "missing"))
	if r.DefaultInterface != "" || len(r.IPv4Routes) != 0 || r.Interfaces[0].Carrier != nil || r.Interfaces[1].SpeedMbps != 0 {
		t.Fatalf("invented hardware facts: %+v", r)
	}
}
