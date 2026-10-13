package device

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/printing"
)

type Status struct {
	Model          string              `json:"model"`
	Kernel         string              `json:"kernel"`
	MemTotalKB     int64               `json:"memTotalKB"`
	MemAvailKB     int64               `json:"memAvailableKB"`
	UptimeSeconds  int64               `json:"uptimeSeconds"`
	LoadOne        float64             `json:"loadOne"`
	Interfaces     []Interface         `json:"interfaces"`
	Disks          []Disk              `json:"disks"`
	Printers       []printing.Printer  `json:"printers"`
	Network        NetworkInspection   `json:"network"`
	Platform       PlatformInspection  `json:"platform"`
	LANPorts       []LANPort           `json:"lanPorts"`
	ARPNeighbors   []ARPNeighbor       `json:"arpNeighbors"`
	Resolvers      ResolverInspection  `json:"resolvers"`
	DHCPLeases     DHCPLeaseEvidence   `json:"dhcpLeases"`
	ServiceSources ServiceDiscovery    `json:"serviceSources"`
	Wireless       WirelessObservation `json:"wireless"`
}
type Interface struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu"`
	Flags     string   `json:"flags"`
	Addresses []string `json:"addresses"`
}
type Disk struct {
	Name       string `json:"name"`
	Bytes      uint64 `json:"bytes"`
	Partition  bool   `json:"partition"`
	Transport  string `json:"transport"`
	Mountpoint string `json:"mountpoint,omitempty"`
	Device     string `json:"device"`
}

func fileText(p string) string {
	b, e := os.ReadFile(p)
	if e != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(b), "\x00", ""))
}
func meminfo(path string) map[string]int64 {
	m := map[string]int64{}
	f, e := os.Open(path)
	if e != nil {
		return m
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		x := strings.Fields(scan.Text())
		if len(x) < 2 {
			continue
		}
		n, _ := strconv.ParseInt(x[1], 10, 64)
		m[strings.TrimSuffix(x[0], ":")] = n
	}
	return m
}
func mounts(path string) map[string]string {
	m := map[string]string{}
	f, e := os.Open(path)
	if e != nil {
		return m
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		v := strings.Fields(s.Text())
		if len(v) >= 2 {
			m[v[0]] = strings.ReplaceAll(v[1], `\040`, " ")
		}
	}
	return m
}
func isUSBPath(p string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.HasPrefix(seg, "usb") {
			if _, e := strconv.Atoi(strings.TrimPrefix(seg, "usb")); e == nil {
				return true
			}
		}
	}
	return false
}

// USBDisks lists only sysfs devices attached to USB ancestors. Read-only.
func USBDisks(sysRoot, mountFile string) []Disk {
	m := mounts(mountFile)
	entries, e := os.ReadDir(sysRoot)
	if e != nil {
		return []Disk{}
	}
	out := []Disk{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "sd") || len(name) < 3 {
			continue
		}
		raw := filepath.Join(sysRoot, name)
		resolved, e := filepath.EvalSymlinks(raw)
		if e != nil || !isUSBPath(resolved) {
			continue
		}
		sectors, e := strconv.ParseUint(fileText(filepath.Join(raw, "size")), 10, 64)
		if e != nil {
			continue
		}
		_, partErr := os.Stat(filepath.Join(raw, "partition"))
		dev := "/dev/" + name
		out = append(out, Disk{Name: name, Bytes: sectors * 512, Partition: partErr == nil, Transport: "USB", Device: dev, Mountpoint: m[dev]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func ValidateUSBPartition(device string) error {
	if !strings.HasPrefix(device, "/dev/sd") {
		return errors.New("only /dev/sd USB partitions allowed")
	}
	for _, d := range USBDisks("/sys/class/block", "/proc/mounts") {
		if d.Device == device && d.Partition {
			return nil
		}
	}
	return fmt.Errorf("%s is not a confirmed USB partition", device)
}
func Snapshot() Status {
	s := Status{Model: fileText("/proc/device-tree/model"), Kernel: fileText("/proc/sys/kernel/osrelease"), Interfaces: []Interface{}, Disks: USBDisks("/sys/class/block", "/proc/mounts"), Printers: printing.List("/sys/class/usb", "/dev")}
	if s.Model == "" {
		s.Model = "Unknown Linux device"
	}
	mi := meminfo("/proc/meminfo")
	s.MemTotalKB, s.MemAvailKB = mi["MemTotal"], mi["MemAvailable"]
	if fields := strings.Fields(fileText("/proc/uptime")); len(fields) > 0 {
		if n, e := strconv.ParseFloat(fields[0], 64); e == nil {
			s.UptimeSeconds = int64(n)
		}
	}
	if fields := strings.Fields(fileText("/proc/loadavg")); len(fields) > 0 {
		if n, e := strconv.ParseFloat(fields[0], 64); e == nil {
			s.LoadOne = n
		}
	}
	ifaces, e := net.Interfaces()
	if e == nil {
		for _, i := range ifaces {
			item := Interface{Name: i.Name, MAC: i.HardwareAddr.String(), MTU: i.MTU, Flags: i.Flags.String(), Addresses: []string{}}
			if adds, err := i.Addrs(); err == nil {
				for _, a := range adds {
					item.Addresses = append(item.Addresses, a.String())
				}
			}
			s.Interfaces = append(s.Interfaces, item)
		}
	}
	sort.Slice(s.Interfaces, func(i, j int) bool { return s.Interfaces[i].Name < s.Interfaces[j].Name })
	s.Network = InspectNetwork(s.Interfaces, "/sys/class/net", "/proc/net/route")
	s.Wireless = SnapshotWireless(s.Model, s.Network)
	s.Platform = SnapshotPlatform()
	s.LANPorts = OperatorLANPorts(s.Model, s.Network.Interfaces)
	s.ARPNeighbors = ReadARPNeighbors("/proc/net/arp")
	s.Resolvers = InspectResolvers("/etc/resolv.conf")
	s.ServiceSources = DiscoverServiceEvidence("/")
	AssociateARP(s.Wireless.Radios, s.ARPNeighbors)
	// Optional standard dnsmasq lease evidence only, never assume a stock
	// proprietary DHCP file or assign an IP based on lease text alone.
	s.DHCPLeases = InspectDHCPLeaseEvidence(s.Wireless.Radios, "/var/lib/misc/dnsmasq.leases", time.Now())
	return s
}
