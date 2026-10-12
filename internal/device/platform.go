package device

import (
	"bufio"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// PlatformInspection contains only observations from the currently running
// Linux kernel and executable PATH, not vendor configuration or VPN state.
type PlatformInspection struct {
	IPv4Forwarding         *bool    `json:"ipv4Forwarding"`
	IPv6Forwarding         *bool    `json:"ipv6Forwarding"`
	IPv4FirewallTables     []string `json:"ipv4FirewallTables"`
	IPv6FirewallTables     []string `json:"ipv6FirewallTables"`
	TUNDevicePresent       bool     `json:"tunDevicePresent"`
	TUNModulePresent       bool     `json:"tunModulePresent"`
	WireGuardModulePresent bool     `json:"wireGuardModulePresent"`
	VPNExecutables         []string `json:"vpnExecutables"`
	Notes                  []string `json:"notes"`
}
type ARPNeighbor struct {
	Address   string `json:"address"`
	MAC       string `json:"mac"`
	Interface string `json:"interface"`
}

// LANPort is an operator-provided board mapping enriched with live carrier
// observations. It is not automatically verified on other RP2613 variants.
type LANPort struct {
	Label           string `json:"label"`
	Interface       string `json:"interface"`
	BridgeInterface string `json:"bridgeInterface"`
	Carrier         *bool  `json:"carrier,omitempty"`
	Master          string `json:"master,omitempty"`
	OperState       string `json:"operState,omitempty"`
	Source          string `json:"source"`
}

func readFlag(path string) *bool {
	switch fileText(path) {
	case "1":
		b := true
		return &b
	case "0":
		b := false
		return &b
	default:
		return nil
	}
}
func boundedLines(path string, limit int) []string {
	f, e := os.Open(path)
	if e != nil {
		return []string{}
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 16384)
	out := []string{}
	for len(out) < limit && s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
func VPNPlatform(root string, lookPath func(string) (string, error)) PlatformInspection {
	p := PlatformInspection{
		IPv4FirewallTables: []string{}, IPv6FirewallTables: []string{}, VPNExecutables: []string{},
		Notes: []string{
			"Stock GRE and IPsec VPN pass-through settings are different from a VPN server or client; HGFW does not read or change them.",
			"The presence of a TUN device or VPN program does not prove a usable tunnel, kernel compatibility or remote connectivity.",
			"No VPN keys, secrets, configuration or active sessions are read or modified.",
		},
	}
	p.IPv4Forwarding = readFlag(filepath.Join(root, "proc/sys/net/ipv4/ip_forward"))
	p.IPv6Forwarding = readFlag(filepath.Join(root, "proc/sys/net/ipv6/conf/all/forwarding"))
	p.IPv4FirewallTables = boundedLines(filepath.Join(root, "proc/net/ip_tables_names"), 16)
	p.IPv6FirewallTables = boundedLines(filepath.Join(root, "proc/net/ip6_tables_names"), 16)
	if st, e := os.Stat(filepath.Join(root, "dev/net/tun")); e == nil && st.Mode()&os.ModeCharDevice != 0 {
		p.TUNDevicePresent = true
	}
	if st, e := os.Stat(filepath.Join(root, "sys/module/tun")); e == nil && st.IsDir() {
		p.TUNModulePresent = true
	}
	if st, e := os.Stat(filepath.Join(root, "sys/module/wireguard")); e == nil && st.IsDir() {
		p.WireGuardModulePresent = true
	}
	for _, bin := range []string{"openvpn", "wireguard-go", "wg", "wg-quick", "ipsec", "strongswan", "pppd"} {
		if _, e := lookPath(bin); e == nil {
			p.VPNExecutables = append(p.VPNExecutables, bin)
		}
	}
	return p
}
func ReadARPNeighbors(path string) []ARPNeighbor {
	lines := boundedLines(path, 129)
	n := []ARPNeighbor{}
	if len(lines) < 2 {
		return n
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if net.ParseIP(fields[0]) == nil {
			continue
		}
		flags, e := strconv.ParseUint(fields[2], 0, 32)
		if e != nil || flags&2 == 0 {
			continue
		}
		mac, e := net.ParseMAC(fields[3])
		if e != nil || len(mac) != 6 {
			continue
		}
		iface := fields[5]
		if len(iface) > 15 || iface == "" {
			continue
		}
		n = append(n, ARPNeighbor{Address: fields[0], MAC: mac.String(), Interface: iface})
	}
	return n
}
func OperatorLANPorts(model string, observed []NetworkEvidence) []LANPort {
	result := []LANPort{}
	// This is evidence contributed by the current HG6245D owner, not Broadcom
	// official hardware documentation. Do not claim an auto-detected port map.
	if !strings.Contains(strings.ToLower(model), "bcm96846") {
		return result
	}
	index := make(map[string]NetworkEvidence, len(observed))
	for _, it := range observed {
		index[it.Name] = it
	}
	for i := 0; i < 4; i++ {
		name := "eth" + strconv.Itoa(i)
		e, ok := index[name]
		if !ok {
			continue
		}
		bridgeName := name + ".0"
		result = append(result, LANPort{Label: "LAN " + strconv.Itoa(i+1), Interface: name, BridgeInterface: bridgeName, Carrier: e.Carrier, OperState: e.OperState, Master: index[bridgeName].Master, Source: "Operator-reported HG6245D RP2613 mapping; verify on other hardware"})
	}
	return result
}
func SnapshotPlatform() PlatformInspection { return VPNPlatform("/", exec.LookPath) }
