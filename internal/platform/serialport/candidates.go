//go:build windows

package serialport

import (
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
