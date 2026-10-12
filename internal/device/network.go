package device

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"regexp"
	"strings"
)

type NetworkConfig struct {
	WANInterface string `json:"wanInterface"`
	LANInterface string `json:"lanInterface"`
	LANAddress   string `json:"lanAddress"`
	DHCPStart    string `json:"dhcpStart"`
	DHCPEnd      string `json:"dhcpEnd"`
	UpstreamMode string `json:"upstreamMode"`
}
type NetworkPlan struct {
	Validated      bool     `json:"validated"`
	Mode           string   `json:"mode"`
	InterfaceRoles []string `json:"interfaceRoles"`
	Notes          []string `json:"notes"`
	AppliesChanges bool     `json:"appliesChanges"`
}

var safeIfName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)

// PlanNetwork validates syntax and IP topology only; NOT driver capabilities.
// A validated plan is never applied to the running Linux network.
func PlanNetwork(c NetworkConfig) (NetworkPlan, error) {
	p := NetworkPlan{Mode: "routed-isolated", AppliesChanges: false}
	for _, name := range []string{c.WANInterface, c.LANInterface} {
		if !safeIfName.MatchString(name) {
			return p, errors.New("invalid interface name")
		}
		if name == "lo" || name == "sfu" || strings.HasPrefix(name, "gpon") || strings.HasPrefix(name, "veip") || strings.HasPrefix(name, "voip") || name == "bcmsw" {
			return p, errors.New("GPON interface cannot be selected")
		}
	}
	if c.WANInterface == c.LANInterface {
		return p, errors.New("WAN and LAN must differ")
	}
	lan, e := netip.ParsePrefix(c.LANAddress)
	if e != nil || !lan.Addr().Is4() || lan.Bits() < 16 || lan.Bits() > 30 {
		return p, errors.New("LAN IPv4 prefix must be /16 through /30")
	}
	start, e1 := netip.ParseAddr(c.DHCPStart)
	end, e2 := netip.ParseAddr(c.DHCPEnd)
	if e1 != nil || e2 != nil || !start.Is4() || !end.Is4() {
		return p, errors.New("invalid IPv4 DHCP range")
	}
	if !lan.Contains(start) || !lan.Contains(end) || start.Compare(end) > 0 || start == lan.Addr() || end == lan.Addr() {
		return p, errors.New("DHCP range conflicts with LAN address or subnet")
	}
	base := lan.Masked().Addr().As4()
	mask := lan.Bits()
	var broadcast [4]byte
	copy(broadcast[:], base[:])
	for bit := mask; bit < 32; bit++ {
		broadcast[bit/8] |= 1 << uint(7-bit%8)
	}
	bcast := netip.AddrFrom4(broadcast)
	if lan.Addr() == lan.Masked().Addr() || lan.Addr() == bcast {
		return p, errors.New("LAN address cannot be network or broadcast address")
	}
	if start == lan.Masked().Addr() || end == bcast || start == bcast || end == lan.Masked().Addr() {
		return p, errors.New("DHCP range cannot contain network or broadcast addresses")
	}
	if c.UpstreamMode != "dhcp" && c.UpstreamMode != "static" {
		return p, errors.New("upstream mode must be dhcp or static")
	}
	p.Validated = true
	p.InterfaceRoles = []string{fmt.Sprintf("WAN %s (%s)", c.WANInterface, c.UpstreamMode), fmt.Sprintf("LAN %s (%s)", c.LANInterface, c.LANAddress), fmt.Sprintf("DHCP %s - %s", c.DHCPStart, c.DHCPEnd)}
	p.Notes = []string{"Syntax and subnet validation only; interface names may not exist on the device.", "Hardware port mapping, upstream support and rollback remain unverified.", "No network, GPON, Wi-Fi, firewall, DNS or DHCP changes were performed."}
	return p, nil
}
func ReadNetworkConfig(path string) (NetworkConfig, error) {
	f, e := os.Open(path)
	if e != nil {
		return NetworkConfig{}, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var c NetworkConfig
	if e = d.Decode(&c); e != nil {
		return NetworkConfig{}, e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return NetworkConfig{}, errors.New("unexpected trailing network plan content")
	}
	return c, nil
}
func ReadNetworkPlan(path string) (NetworkPlan, error) {
	c, e := ReadNetworkConfig(path)
	if e != nil {
		return NetworkPlan{}, e
	}
	return PlanNetwork(c)
}

// PlanAgainstInventory additionally verifies referenced interfaces are actually
// present in an observed snapshot. This is NOT permission to apply the plan.
func PlanAgainstInventory(c NetworkConfig, interfaces []Interface) (NetworkPlan, error) {
	p, err := PlanNetwork(c)
	if err != nil {
		return p, err
	}
	seen := map[string]bool{}
	for _, i := range interfaces {
		seen[i.Name] = true
	}
	if !seen[c.WANInterface] || !seen[c.LANInterface] {
		p.Validated = false
		return p, errors.New("WAN or LAN interface not present in current kernel inventory")
	}
	p.Notes = append(p.Notes, "Names present in current kernel inventory; physical jack mapping and service readiness still unverified.")
	return p, nil
}
