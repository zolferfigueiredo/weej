package core

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type PortInfo struct {
	Name    string
	IsUSB   bool
	VID     string
	PID     string
	Product string
}

var knownVIDOrder = []string{"2341", "2A03", "1A86", "10C4", "0403"}

func vidRank(vid string) int {
	for i, v := range knownVIDOrder {
		if strings.EqualFold(v, vid) {
			return i
		}
	}
	return len(knownVIDOrder)
}

func comNumber(name string) int {
	upper := strings.ToUpper(name)
	idx := strings.Index(upper, "COM")
	if idx < 0 {
		return math.MaxInt
	}
	n, err := strconv.Atoi(upper[idx+3:])
	if err != nil {
		return math.MaxInt
	}
	return n
}

func CandidatePorts(ports []PortInfo) []string {
	var usb []PortInfo
	for _, p := range ports {
		if p.IsUSB {
			usb = append(usb, p)
		}
	}
	sort.SliceStable(usb, func(i, j int) bool {
		ri, rj := vidRank(usb[i].VID), vidRank(usb[j].VID)
		if ri != rj {
			return ri < rj
		}
		return comNumber(usb[i].Name) < comNumber(usb[j].Name)
	})
	names := make([]string, len(usb))
	for i, p := range usb {
		names[i] = p.Name
	}
	return names
}
