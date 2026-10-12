package device

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
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

// PlanNetwork prepares an isolated routed topology, but makes NO changes to Linux interfaces.
func PlanNetwork(c NetworkConfig) (NetworkPlan, error) {
	p := NetworkPlan{Mode: "routed-isolated", AppliesChanges: false}
	for _, name := range []string{c.WANInterface, c.LANInterface} {
		if name == "" || strings.ContainsAny(name, " \t\n/\\;") {
			return p, errors.New("invalid interface name")
		}
		if name == "sfu" || strings.HasPrefix(name, "gpon") {
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
	if c.UpstreamMode != "dhcp" && c.UpstreamMode != "static" {
		return p, errors.New("upstream mode must be dhcp or static")
	}
	p.Validated = true
	p.InterfaceRoles = []string{fmt.Sprintf("WAN %s (%s)", c.WANInterface, c.UpstreamMode), fmt.Sprintf("LAN %s (%s)", c.LANInterface, c.LANAddress), fmt.Sprintf("DHCP %s - %s", c.DHCPStart, c.DHCPEnd)}
	p.Notes = []string{"This is a non-mutating plan; do not apply without verified hardware switch map and rollback.", "Do not bridge GPON sfu with Wi-Fi station or LAN.", "Firewall, DNS and DHCP daemons must be separately deployed and validated."}
	return p, nil
}
func ReadNetworkPlan(path string) (NetworkPlan, error) {
	f, e := os.Open(path)
	if e != nil {
		return NetworkPlan{}, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var c NetworkConfig
	if e = d.Decode(&c); e != nil {
		return NetworkPlan{}, e
	}
	return PlanNetwork(c)
}
