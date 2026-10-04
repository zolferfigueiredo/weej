//go:build windows

package serialport

import (
	"sort"
	"strconv"
	"strings"

	"github.com/zolferfigueiredo/weej/internal/core"

	"go.bug.st/serial/enumerator"
)

// pickCandidates lists ports worth trying to open. A forced port is used as given, bypassing
// enumeration entirely, per the spec: only that port, no fallback.
func pickCandidates(forcedPort string) []string {
	if forcedPort != "" {
		return []string{forcedPort}
	}

	// No filters: this skips active USB probing, which the library warns can interfere with
	// device operation. VID/PID/IsUSB don't need it.
	details, err := enumerator.GetDetailedPortsList()
	if err != nil || len(details) == 0 {
		return nil
	}

	infos := make([]core.PortInfo, len(details))
	for i, d := range details {
		infos[i] = core.PortInfo{
			Name:    d.Name,
			IsUSB:   d.IsUSB,
			VID:     d.VID,
			PID:     d.PID,
			Product: d.Product,
		}
	}
	return core.CandidatePorts(infos)
}

// PortInfo describes one serial port for the Settings list.
type PortInfo struct {
	Name    string `json:"name"`
	Product string `json:"product"`
	USB     bool   `json:"usb"`
}

// List returns every serial port Windows knows about, in COM number order.
func List() []PortInfo {
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil
	}
	out := make([]PortInfo, 0, len(details))
	for _, d := range details {
		// Windows names ports like "USB-SERIAL CH340 (COM6)"; the list shows the COM name already.
		product := strings.TrimSpace(strings.TrimSuffix(d.Product, "("+d.Name+")"))
		out = append(out, PortInfo{Name: d.Name, Product: product, USB: d.IsUSB})
	}
	sort.Slice(out, func(i, j int) bool { return comNumber(out[i].Name) < comNumber(out[j].Name) })
	return out
}

func comNumber(name string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(name), "COM"))
	if err != nil {
		return 1 << 30
	}
	return n
}
