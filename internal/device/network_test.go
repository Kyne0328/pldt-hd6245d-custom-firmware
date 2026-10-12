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
