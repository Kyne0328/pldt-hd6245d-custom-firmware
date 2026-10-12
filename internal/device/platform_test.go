package device

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformObservationSafe(t *testing.T) {
	root := t.TempDir()
	write := func(name, value string) {
		path := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, []byte(value), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("proc/sys/net/ipv4/ip_forward", "1\n")
	write("proc/sys/net/ipv6/conf/all/forwarding", "0\n")
	write("proc/net/ip_tables_names", "filter\nnat\n")
	write("proc/net/arp", "IP address       HW type     Flags       HW address            Mask     Device\n192.168.1.10 0x1 0x2 aa:bb:cc:dd:ee:ff * br0\n192.168.1.11 0x1 0x0 00:00:00:00:00:00 * br0\n")
	if e := os.MkdirAll(filepath.Join(root, "sys/module/tun"), 0700); e != nil {
		t.Fatal(e)
	}
	platform := VPNPlatform(root, func(name string) (string, error) {
		if name == "wg" {
			return "/test/wg", nil
		}
		return "", os.ErrNotExist
	})
	if platform.IPv4Forwarding == nil || !*platform.IPv4Forwarding || platform.IPv6Forwarding == nil || *platform.IPv6Forwarding {
		t.Fatal("kernel flags incorrectly observed")
	}
	if !platform.TUNModulePresent || platform.TUNDevicePresent || platform.WireGuardModulePresent {
		t.Fatal("capabilities fabricated")
	}
	if len(platform.IPv4FirewallTables) != 2 || len(platform.VPNExecutables) != 1 || platform.VPNExecutables[0] != "wg" {
		t.Fatal("unexpected inventory", platform)
	}
	neighbors := ReadARPNeighbors(filepath.Join(root, "proc/net/arp"))
	if len(neighbors) != 1 || neighbors[0].Address != "192.168.1.10" || neighbors[0].Interface != "br0" {
		t.Fatalf("invalid ARP parsing: %+v", neighbors)
	}
}
func TestNoKernelVPNMisrepresented(t *testing.T) {
	p := VPNPlatform(t.TempDir(), func(string) (string, error) { return "", os.ErrNotExist })
	if p.IPv4Forwarding != nil || p.TUNDevicePresent || p.TUNModulePresent || p.WireGuardModulePresent || len(p.VPNExecutables) != 0 || len(p.IPv4FirewallTables) != 0 {
		t.Fatalf("unknown incorrectly interpreted: %+v", p)
	}
}
func TestOperatorLANPortMapping(t *testing.T) {
	yes := true
	no := false
	e := []NetworkEvidence{{Name: "eth0", Carrier: &yes}, {Name: "eth1", Carrier: &no}, {Name: "eth0.0", Master: "br0"}, {Name: "eth1.0", Master: "br0"}}
	p := OperatorLANPorts("Broadcom BCM96846", e)
	if len(p) != 2 || p[0].Label != "LAN 1" || p[1].Label != "LAN 2" || p[0].Master != "br0" || p[1].Carrier == nil || *p[1].Carrier || !strings.Contains(p[0].Source, "Operator") {
		t.Fatalf("bad evidence map: %+v", p)
	}
	if len(OperatorLANPorts("Unknown Linux device", e)) != 0 {
		t.Fatal("invented board mapping")
	}
}
