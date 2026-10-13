// Package fieldcheck performs a privacy-safe, single-run hardware verification
// of independent HGFW features. It never serializes SSIDs, MACs, IP addresses,
// credentials, routes, DHCP leases, or client identifiers.
package fieldcheck

import (
	"errors"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/device"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/dhcpcore"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/dnscore"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/netprobe"
	"github.com/Kyne0328/pldt-hd6245d-custom-firmware/internal/smoke"
)

type Inventory struct {
	Interfaces                  int      `json:"interfaces"`
	Routes                      int      `json:"ipv4Routes"`
	HasDefaultRoute             bool     `json:"hasDefaultIPv4Route"`
	ResolverFileReadable        bool     `json:"resolverFileReadable"`
	ResolverCount               int      `json:"resolverCount"`
	ARPNeighbors                int      `json:"arpNeighborCount"`
	MatchedWirelessARP          int      `json:"wirelessARPMatches"`
	LeaseFileObserved           bool     `json:"standardLeaseFileObserved"`
	LeaseRows                   int      `json:"standardLeaseRows"`
	ResolverCandidatePaths      int      `json:"resolverCandidatePathsPresent"`
	LeaseCandidatePaths         int      `json:"leaseCandidatePathsPresent"`
	VendorDHCPConfigPaths       int      `json:"vendorDHCPConfigPathsPresent"`
	ResolverSymlinkCandidates   int      `json:"resolverSymlinkCandidates"`
	ResolverRegularFiles        int      `json:"resolverRegularFiles"`
	VendorUDHCPDLeasePresent    bool     `json:"vendorUDHCPDLeasePresent"`
	KnownServiceProcessNames    []string `json:"knownServiceProcessNames"`
	DNSUDP53Observed            bool     `json:"dnsUDP53Observed"`
	DNSTCP53Listening           bool     `json:"dnsTCP53Listening"`
	DHCPUDP67Observed           bool     `json:"dhcpUDP67Observed"`
	WirelessLeaseNameMatches    int      `json:"wirelessLeaseNameMatches"`
	WirelessRadios              int      `json:"radios"`
	AvailableAssociationQueries int      `json:"availableAssociationQueries"`
	AssociatedStations          int      `json:"associatedStations"`
	StationsWithSignal          int      `json:"stationsWithSignal"`
	StationsWithRX              int      `json:"stationsWithRX"`
	StationsWithTX              int      `json:"stationsWithTX"`
	USBBlockDevices             int      `json:"usbBlockDevices"`
	USBPrinters                 int      `json:"usbPrinters"`
}
type Result struct {
	Success                       bool      `json:"success"`
	ReadOnly                      bool      `json:"readOnly"`
	Inventory                     Inventory `json:"inventory"`
	DNSLoopbackPassed             bool      `json:"dnsLoopbackPassed"`
	DHCPPacketEnginePassed        bool      `json:"dhcpPacketEnginePassed"`
	SupervisedLoopbackPassed      bool      `json:"supervisedLoopbackPassed"`
	AuthenticatedAdminSmokePassed bool      `json:"authenticatedAdminSmokePassed"`
	FailedChecks                  []string  `json:"failedChecks"`
	Notes                         []string  `json:"notes"`
}

func Summarize(s device.Status) Inventory {
	v := Inventory{Interfaces: len(s.Interfaces), Routes: len(s.Network.IPv4Routes),
		HasDefaultRoute: s.Network.DefaultInterface != "", ResolverFileReadable: s.Resolvers.Available,
		ResolverCount: len(s.Resolvers.Nameservers), ARPNeighbors: len(s.ARPNeighbors),
		USBBlockDevices: len(s.Disks), USBPrinters:len(s.Printers), WirelessRadios: len(s.Wireless.Radios),
		LeaseFileObserved: s.DHCPLeases.Available, LeaseRows: s.DHCPLeases.ValidRows,
		WirelessLeaseNameMatches: s.DHCPLeases.MatchedWirelessNames,
		KnownServiceProcessNames: s.ServiceSources.KnownProcessNames,
		DNSUDP53Observed:         s.ServiceSources.DNSUDP53Observed,
		DNSTCP53Listening:        s.ServiceSources.DNSTCP53Listening,
		DHCPUDP67Observed:        s.ServiceSources.DHCPUDP67Observed}
	for _, x := range s.ServiceSources.ResolverCandidates {
		if x.Present {
			v.ResolverCandidatePaths++
			if x.Kind == "symlink (target not inspected)" {
				v.ResolverSymlinkCandidates++
			}
			if x.Kind == "regular file" {
				v.ResolverRegularFiles++
			}
		}
	}
	for _, x := range s.ServiceSources.LeaseCandidates {
		if x.Present {
			v.LeaseCandidatePaths++
			if x.Label == "/var/udhcpd/udhcpd.leases" {
				v.VendorUDHCPDLeasePresent = true
			}
		}
	}
	for _, x := range s.ServiceSources.VendorDHCPConfigCandidates {
		if x.Present {
			v.VendorDHCPConfigPaths++
		}
	}
	for _, r := range s.Wireless.Radios {
		if r.AssociationStatus != "observed" {
			continue
		}
		v.AvailableAssociationQueries++
		v.AssociatedStations += len(r.Stations)
		for _, st := range r.Stations {
			if st.SignalDBm != nil {
				v.StationsWithSignal++
			}
			if st.RXBitrate != "" {
				v.StationsWithRX++
			}
			if st.TXBitrate != "" {
				v.StationsWithTX++
			}
			if st.NeighborIPv4 != "" {
				v.MatchedWirelessARP++
			}
		}
	}
	return v
}
func Run() (Result, error) {
	out := Result{ReadOnly: true, FailedChecks: []string{}, Notes: []string{
		"Aggregate counts only: no station addresses, SSIDs, IPs or passwords are printed.",
		"ARP matches are kernel neighbor evidence; optional lease names require agreement across associations, ARP and lease rows and do not prove identity.",
		"Fixed-path and service-port presence is diagnostic only, not proof of DHCP/DNS ownership or functionality.",
		"DHCP packet processing is in memory only, without UDP/67, broadcasting or serving client requests.",
		"DNS server test listens only on temporary 127.0.0.1 UDP, never the LAN or port 53.",
		"Loopback HTTP checks stop their own temporary services; no kernel, flash or stock configuration is modified.",
	}}
	out.Inventory = Summarize(device.Snapshot())
	dhcp, derr := dhcpcore.SelfTest()
	out.DHCPPacketEnginePassed = derr == nil && dhcp.Success
	if !out.DHCPPacketEnginePassed {
		out.FailedChecks = append(out.FailedChecks, "dhcp-packet-engine")
	}
	d, err := dnscore.SelfTest()
	out.DNSLoopbackPassed = err == nil && d.Success
	if !out.DNSLoopbackPassed {
		out.FailedChecks = append(out.FailedChecks, "dns-loopback")
	}
	n, nerr := netprobe.SelfTest()
	out.SupervisedLoopbackPassed = nerr == nil && n.Success
	if !out.SupervisedLoopbackPassed {
		out.FailedChecks = append(out.FailedChecks, "supervised-loopback")
	}
	a, aerr := smoke.Run()
	out.AuthenticatedAdminSmokePassed = aerr == nil && a.Success
	if !out.AuthenticatedAdminSmokePassed {
		out.FailedChecks = append(out.FailedChecks, "authenticated-admin")
	}
	out.Success = len(out.FailedChecks) == 0
	if !out.Success {
		return out, errors.New("one or more field checks failed; refer to failedChecks")
	}
	return out, nil
}
