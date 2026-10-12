package device

import (
	"net/netip"
	"strings"
)

// AssociateARP attaches a kernel-neighbor IPv4 observation only to an already
// authenticated driver association. ARP is neither a DHCP lease nor proof of
// current reachability, and a missing/conflicting observation stays unknown.
func AssociateARP(radios []WirelessRadio, neighbors []ARPNeighbor) {
	for ri := range radios {
		radio := &radios[ri]
		if radio.AssociationStatus != "observed" {
			continue
		}
		for si := range radio.Stations {
			st := &radio.Stations[si]
			mac := normalMAC(st.MAC)
			if mac == "" {
				continue
			}
			var ip, iface string
			ambiguous := false
			for _, n := range neighbors {
				if normalMAC(n.MAC) != mac {
					continue
				}
				// The link must be observed on this radio or its kernel bridge master.
				if n.Interface != radio.Interface && (radio.Master == "" || n.Interface != radio.Master) {
					continue
				}
				addr, err := netip.ParseAddr(n.Address)
				if err != nil || !addr.Is4() || addr.IsUnspecified() || addr.IsMulticast() {
					continue
				}
				if ip != "" && (ip != addr.String() || iface != n.Interface) {
					ambiguous = true
					break
				}
				ip, iface = addr.String(), n.Interface
			}
			if ip != "" && !ambiguous {
				st.NeighborIPv4 = ip
				st.NeighborInterface = iface
			}
		}
	}
}

// CountARPLinks reports observed matches, not DHCP leases or unique people.
func CountARPLinks(radios []WirelessRadio) int {
	count := 0
	for _, r := range radios {
		for _, st := range r.Stations {
			if strings.TrimSpace(st.NeighborIPv4) != "" {
				count++
			}
		}
	}
	return count
}
