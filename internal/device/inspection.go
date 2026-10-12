package device

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NetworkEvidence is observations only, not a hardware port map. Linux
// interface names do not establish which plastic Ethernet jack they use.
type NetworkEvidence struct {
	Name       string `json:"name"`
	Role       string `json:"role"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	RoleNote   string `json:"roleNote"`
	OperState  string `json:"operState,omitempty"`
	Carrier    *bool  `json:"carrier,omitempty"`
	Master     string `json:"master,omitempty"`
	SpeedMbps  int64  `json:"speedMbps,omitempty"`
	RXBytes    uint64 `json:"rxBytes,omitempty"`
	TXBytes    uint64 `json:"txBytes,omitempty"`
}
type IPv4Route struct {
	Interface   string `json:"interface"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Mask        string `json:"mask"`
	Metric      uint64 `json:"metric"`
	Default     bool   `json:"default"`
}
type NetworkInspection struct {
	Interfaces       []NetworkEvidence `json:"interfaces"`
	IPv4Routes       []IPv4Route       `json:"ipv4Routes"`
	DefaultInterface string            `json:"defaultInterface,omitempty"`
	DefaultGateway   string            `json:"defaultGateway,omitempty"`
	Notes            []string          `json:"notes"`
}

func classifyNetwork(name string) (role, kind, confidence, note string) {
	switch {
	case name == "lo":
		return "Loopback", "system", "verified", "Local host interface; not a physical port."
	case strings.HasPrefix(name, "br") && !strings.HasPrefix(name, "brport"):
		return "Linux bridge", "system", "inferred", "Bridge name or kernel bridge metadata; membership must be confirmed."
	case name == "wl0" || name == "wl1":
		return "Broadcom wireless radio interface", "wireless", "name-only", "Driver interface detected; AP/STA capability and RF operation not verified."
	case strings.HasPrefix(name, "wl"):
		return "Broadcom wireless virtual interface", "wireless", "name-only", "Virtual Wi-Fi interface; SSID assignment and radio state unknown."
	case strings.HasPrefix(name, "eth"):
		return "Broadcom Ethernet/logical interface", "ethernet", "name-only", "Cannot map this interface to a numbered LAN jack from its name."
	case strings.HasPrefix(name, "wan"):
		return "Vendor-named WAN interface", "ethernet", "name-only", "A name is not proof of an active upstream or physical WAN port."
	case name == "bcmsw":
		return "Broadcom switch control", "system", "name-only", "Switch device; port/VLAN register mapping not verified."
	case name == "sfu" || strings.HasPrefix(name, "gpon") || strings.HasPrefix(name, "veip"):
		return "Optical/vendor interface", "system", "name-only", "GPON/OMCI functions are outside independently verified control."
	default:
		return "System / vendor interface", "system", "unknown", "Role not established from available read-only evidence."
	}
}
func readUint(path string) uint64 {
	n, e := strconv.ParseUint(fileText(path), 10, 64)
	if e != nil {
		return 0
	}
	return n
}
func InspectNetwork(interfaces []Interface, sysNetRoot, routeFile string) NetworkInspection {
	result := NetworkInspection{Interfaces: []NetworkEvidence{}, IPv4Routes: []IPv4Route{}, Notes: []string{
		"Interface labels are evidence-based estimates, not a verified physical LAN jack or WAN port map.",
		"Carrier and operstate are Linux driver reports, not verified internet connectivity.",
		"Wi-Fi interface presence does not establish AP, client or repeater operating mode.",
		"Inspection is read-only: no routes, bridges, VLANs, interfaces or firewall rules are changed.",
	}}
	for _, i := range interfaces {
		role, kind, conf, note := classifyNetwork(i.Name)
		base := filepath.Join(sysNetRoot, i.Name)
		e := NetworkEvidence{Name: i.Name, Role: role, Kind: kind, Confidence: conf, RoleNote: note, OperState: fileText(filepath.Join(base, "operstate")),
			RXBytes: readUint(filepath.Join(base, "statistics", "rx_bytes")), TXBytes: readUint(filepath.Join(base, "statistics", "tx_bytes"))}
		carrier := fileText(filepath.Join(base, "carrier"))
		if carrier == "1" || carrier == "0" {
			value := carrier == "1"
			e.Carrier = &value
		}
		if speed, err := strconv.ParseInt(fileText(filepath.Join(base, "speed")), 10, 64); err == nil && speed > 0 && speed < 100000 {
			e.SpeedMbps = speed
		}
		if target, err := filepath.EvalSymlinks(filepath.Join(base, "master")); err == nil {
			e.Master = filepath.Base(target)
		}
		// The existence of a kernel bridge folder is stronger than its name alone.
		if _, err := os.Stat(filepath.Join(base, "bridge")); err == nil {
			e.Role = "Linux bridge"
			e.Confidence = "kernel-sysfs"
			e.Kind = "system"
			e.RoleNote = "Kernel exposes a bridge directory; attached devices may still require verification."
		}
		result.Interfaces = append(result.Interfaces, e)
	}
	result.IPv4Routes = ReadIPv4Routes(routeFile)
	for _, r := range result.IPv4Routes {
		if r.Default && (result.DefaultInterface == "" || r.Metric < routeMetric(result.IPv4Routes, result.DefaultInterface)) {
			result.DefaultInterface = r.Interface
			result.DefaultGateway = r.Gateway
		}
	}
	return result
}
func routeMetric(list []IPv4Route, name string) uint64 {
	for _, r := range list {
		if r.Default && r.Interface == name {
			return r.Metric
		}
	}
	return ^uint64(0)
}
func hexIPv4(x string) (string, error) {
	n, e := strconv.ParseUint(x, 16, 32)
	if e != nil {
		return "", e
	}
	return net.IPv4(byte(n), byte(n>>8), byte(n>>16), byte(n>>24)).String(), nil
}

// ReadIPv4Routes reads /proc/net/route, ignoring malformed or inactive lines.
func ReadIPv4Routes(path string) []IPv4Route {
	routes := []IPv4Route{}
	f, err := os.Open(path)
	if err != nil {
		return routes
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return routes
	}
	for scanner.Scan() {
		v := strings.Fields(scanner.Text())
		if len(v) < 8 {
			continue
		}
		flags, e := strconv.ParseUint(v[3], 16, 32)
		if e != nil || flags&1 == 0 {
			continue
		}
		dest, ed := hexIPv4(v[1])
		gate, eg := hexIPv4(v[2])
		mask, em := hexIPv4(v[7])
		metric, et := strconv.ParseUint(v[6], 10, 64)
		if ed != nil || eg != nil || em != nil || et != nil {
			continue
		}
		isDefault := dest == "0.0.0.0" && mask == "0.0.0.0"
		route := IPv4Route{Interface: v[0], Destination: dest, Mask: mask, Metric: metric, Default: isDefault}
		if gate != "0.0.0.0" {
			route.Gateway = gate
		}
		routes = append(routes, route)
	}
	return routes
}
func (n NetworkInspection) Summary() string {
	if n.DefaultInterface == "" {
		return "No IPv4 default route reported"
	}
	return fmt.Sprintf("Default IPv4 route uses %s via %s (driver and reachability unverified)", n.DefaultInterface, n.DefaultGateway)
}
