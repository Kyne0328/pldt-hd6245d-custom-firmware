package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkPlan(t *testing.T) {
	c := NetworkConfig{WANInterface: "wl1", LANInterface: "wl0", LANAddress: "192.168.50.1/24", DHCPStart: "192.168.50.100", DHCPEnd: "192.168.50.200", UpstreamMode: "dhcp"}
	p, e := PlanNetwork(c)
	if e != nil || !p.Validated || p.AppliesChanges {
		t.Fatal("plan failure", p, e)
	}
	c.WANInterface = "sfu"
	if _, e = PlanNetwork(c); e == nil {
		t.Fatal("GPON sfu allowed")
	}
	c.WANInterface = "wl1"
	c.LANInterface = "wl1"
	if _, e = PlanNetwork(c); e == nil {
		t.Fatal("overlapping interfaces accepted")
	}
	c.LANInterface = "wl0"
	c.DHCPStart = "192.168.1.100"
	if _, e = PlanNetwork(c); e == nil {
		t.Fatal("outside-subnet DHCP accepted")
	}
}
func TestNetworkPlanHardwareGuardAndReservedAddress(t *testing.T) {
	base := NetworkConfig{WANInterface: "eth0", LANInterface: "br-lan", LANAddress: "192.168.50.1/24", DHCPStart: "192.168.50.100", DHCPEnd: "192.168.50.200", UpstreamMode: "dhcp"}
	cfg := base
	cfg.DHCPEnd = "192.168.50.255"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("broadcast DHCP allowed")
	}
	cfg = base
	cfg.DHCPStart = "192.168.50.0"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("network DHCP address allowed")
	}
	cfg = base
	cfg.LANAddress = "192.168.50.0/24"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("network LAN address accepted")
	}
	cfg = base
	cfg.WANInterface = "lo"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("loopback accepted as WAN")
	}
	cfg = base
	cfg.WANInterface = "veip0"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("GPON virtual interface allowed")
	}
	cfg = base
	cfg.WANInterface = "eth0;rm -rf"
	if _, e := PlanNetwork(cfg); e == nil {
		t.Fatal("bad interface token allowed")
	}
	cfg = base
	if _, e := PlanAgainstInventory(cfg, []Interface{{Name: "eth0"}}); e == nil {
		t.Fatal("nonexistent LAN accepted")
	}
	p, e := PlanAgainstInventory(cfg, []Interface{{Name: "eth0"}, {Name: "br-lan"}})
	if e != nil || !p.Validated || p.AppliesChanges {
		t.Fatalf("plan wrongly rejected: %+v %v", p, e)
	}
	planPath := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(planPath, []byte(`{"wanInterface":"eth0","lanInterface":"br-lan"}{"unknown":true}`), 0600)
	if _, e := ReadNetworkConfig(planPath); e == nil {
		t.Fatal("trailing JSON accepted")
	}
}
func TestUSBDeviceFilter(t *testing.T) {
	d := t.TempDir()
	usb := filepath.Join(d, "sys", "devices", "usb1", "1-1")
	if e := os.MkdirAll(filepath.Join(usb, "sda1"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(usb, "sda1", "size"), []byte("1024\n"), 0644); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(usb, "sda1", "partition"), []byte("1\n"), 0644); e != nil {
		t.Fatal(e)
	}
	block := filepath.Join(d, "block")
	os.Mkdir(block, 0755)
	if e := os.Symlink(filepath.Join(usb, "sda1"), filepath.Join(block, "sda1")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(d, "fake"), filepath.Join(block, "sdb1")); e != nil {
		t.Fatal(e)
	}
	mounts := filepath.Join(d, "mounts")
	os.WriteFile(mounts, []byte("/dev/sda1 /mnt/stick vfat ro 0 0\n"), 0644)
	list := USBDisks(block, mounts)
	if len(list) != 1 || list[0].Bytes != 524288 || !list[0].Partition || list[0].Mountpoint != "/mnt/stick" {
		t.Fatalf("USB filtering failed: %+v", list)
	}
}
