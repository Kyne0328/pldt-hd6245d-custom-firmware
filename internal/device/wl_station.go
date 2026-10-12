package device

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Broadcom wl's AP-side per-station commands accept only the associated MAC.
// Queries run after the complete radio inventory so a slow or unsupported
// driver cannot starve wl1's association list. No radio setting is changed.
const maxWLDetailStations = 8
const wlDetailBudget = 2500 * time.Millisecond

var wlRSSIPattern = regexp.MustCompile(`(?i)^\s*(?:rssi\s*:\s*)?(-?[0-9]{1,3})\s*(?:dBm)?\s*$`)
var wlPacketRatePattern = regexp.MustCompile(`(?i)^rate of last (tx|rx) pkt:\s*([0-9]+(?:\.[0-9]+)?)\s*(k|m|g)bps$`)

func parseWLRSSI(raw string) *int {
	m := wlRSSIPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 2 {
		return nil
	}
	dbm, err := strconv.Atoi(m[1])
	if err != nil || dbm < -130 || dbm >= 0 {
		return nil
	}
	return &dbm
}

// parseWLStationRates uses packet rates, not the station's supported rateset.
// RX/TX are from the AP/router perspective. Unsupported driver fields remain
// blank. Antenna-specific RSSI is not collapsed into a fabricated total RSSI.
func parseWLStationRates(output string) (rx, tx string) {
	for _, line := range strings.Split(output, "\n") {
		match := wlPacketRatePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 4 {
			continue
		}
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(match[3]) {
		case "k":
			value /= 1000
		case "g":
			value *= 1000
		}
		if value <= 0 || value > 10000 {
			continue
		}
		formatted := strconv.FormatFloat(value, 'f', -1, 64) + " MBit/s"
		if strings.EqualFold(match[1], "tx") {
			tx = formatted
		} else {
			rx = formatted
		}
	}
	return rx, tx
}

func enrichWLStations(radios []WirelessRadio, run wifiRunner) {
	ctx, cancel := context.WithTimeout(context.Background(), wlDetailBudget)
	defer cancel()
	queried := 0
	// Round-robin avoids starving 5 GHz when the 2.4 GHz radio has many stations.
	for stationIndex := 0; stationIndex < 64 && queried < maxWLDetailStations; stationIndex++ {
		for radioIndex := range radios {
			radio := &radios[radioIndex]
			if queried >= maxWLDetailStations || ctx.Err() != nil {
				return
			}
			if radio.Source != "Broadcom wl assoclist" || stationIndex >= len(radio.Stations) {
				continue
			}
			st := &radio.Stations[stationIndex]
			if normalMAC(st.MAC) != st.MAC {
				continue
			}
			queried++
			if output, err := wifiCallContext(ctx, run, "wl", "-i", radio.Interface, "sta_info", st.MAC); err == nil {
				st.RXBitrate, st.TXBitrate = parseWLStationRates(output)
				if st.RXBitrate != "" || st.TXBitrate != "" {
					st.Source += " + wl sta_info"
				}
			}
			if ctx.Err() != nil {
				return
			}
			if output, err := wifiCallContext(ctx, run, "wl", "-i", radio.Interface, "rssi", st.MAC); err == nil {
				st.SignalDBm = parseWLRSSI(output)
				if st.SignalDBm != nil {
					st.Source += " + wl rssi"
				}
			}
		}
	}
}
