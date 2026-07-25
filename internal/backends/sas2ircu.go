// Package backends holds topo.Backend implementations, one per drive source.
package backends

import (
	"fmt"
	"strings"

	"github.com/nicolerenee/drive-topology-exporter/internal/linux"
	"github.com/nicolerenee/drive-topology-exporter/internal/sas2ircu"
	"github.com/nicolerenee/drive-topology-exporter/internal/topo"
)

// SAS2IRCU discovers drives behind LSI/Broadcom SAS-2 HBAs via the `sas2ircu`
// CLI. It correlates each reported slot to its Linux device by serial/WWN.
type SAS2IRCU struct {
	parser *sas2ircu.Parser
}

// NewSAS2IRCU builds the backend; path overrides sas2ircu binary autodetection.
func NewSAS2IRCU(path string) *SAS2IRCU {
	return &SAS2IRCU{parser: sas2ircu.NewParserWithPath(path)}
}

func (s *SAS2IRCU) Name() string { return "sas2ircu" }

// Available is true when `sas2ircu LIST` succeeds (tool present + HBA visible).
func (s *SAS2IRCU) Available() bool {
	_, err := s.parser.ListControllers()
	return err == nil
}

func (s *SAS2IRCU) Collect() (topo.Snapshot, error) {
	controllers, err := s.parser.ListControllers()
	if err != nil {
		return topo.Snapshot{}, fmt.Errorf("sas2ircu LIST: %w", err)
	}
	var snap topo.Snapshot
	for _, c := range controllers {
		details, err := s.parser.DisplayController(c.Index)
		if err != nil {
			return snap, fmt.Errorf("sas2ircu %d DISPLAY: %w", c.Index, err)
		}
		ctrl := fmt.Sprintf("%d", c.Index)

		logicalID := make(map[int]string, len(details.Enclosures))
		for _, e := range details.Enclosures {
			logicalID[e.EnclosureNum] = e.LogicalID
			snap.Enclosures = append(snap.Enclosures, topo.Enclosure{
				Backend:     s.Name(),
				Controller:  ctrl,
				Enclosure:   fmt.Sprintf("%d", e.EnclosureNum),
				EnclosureID: e.LogicalID,
				NumSlots:    e.NumSlots,
				StartSlot:   e.StartSlot,
			})
		}

		for _, d := range details.PhysicalDevices {
			if !isDisk(d) {
				continue // skip enclosure-services processors, expanders, etc.
			}
			snap.Drives = append(snap.Drives, topo.Drive{
				Backend:     s.Name(),
				Controller:  ctrl,
				Enclosure:   fmt.Sprintf("%d", d.EnclosureNum),
				EnclosureID: logicalID[d.EnclosureNum],
				Slot:        d.SlotNum,
				Serial:      d.SerialNo,
				Model:       d.ModelNumber,
				WWN:         d.GUID,
				SASAddress:  d.SASAddress,
				LinuxDevice: resolveLinuxDevice(d),
				Protocol:    d.Protocol,
				DriveType:   d.DriveType,
				State:       d.State,
				SizeBytes:   d.SizeMB * 1024 * 1024,
			})
		}
	}
	return snap, nil
}

// isDisk keeps hard/solid-state disks and drops enclosure-services devices.
func isDisk(d sas2ircu.PhysicalDevice) bool {
	if strings.Contains(strings.ToLower(d.DeviceType), "disk") {
		return true
	}
	// Fallback for firmware that omits a device-type marker: real drives carry
	// a serial; SES processors generally do not.
	return d.DeviceType == "" && d.SerialNo != ""
}

// resolveLinuxDevice maps a sas2ircu drive to /dev/<name> via serial then WWN.
func resolveLinuxDevice(d sas2ircu.PhysicalDevice) string {
	if d.SerialNo != "" {
		if di, _ := linux.GetDiskInfoBySerial(d.SerialNo); di != nil {
			return di.DeviceName
		}
	}
	if d.GUID != "" {
		if di, _ := linux.GetDiskInfoByGUID(d.GUID); di != nil {
			return di.DeviceName
		}
	}
	return ""
}
