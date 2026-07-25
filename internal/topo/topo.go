// Package topo defines the backend-agnostic drive-topology model.
//
// A Backend discovers the *detectable* physical facts about drives — which
// enclosure slot a drive sits in, its identity, and the OS device it maps to.
// Everything opinion-shaped (friendly enclosure names, rack U, row/col geometry)
// deliberately lives in the consuming inventory/UI, never here.
package topo

// Drive is one physical drive located in an enclosure slot.
type Drive struct {
	Backend     string // discovering backend, e.g. "sas2ircu"
	Controller  string // HBA index/id
	Enclosure   string // enclosure number as reported (may renumber across reboots)
	EnclosureID string // STABLE enclosure identifier (SES logical ID / SAS address)
	Slot        int    // bay number within the enclosure
	Serial      string // drive serial — join key to smartctl
	Model       string
	WWN         string // world-wide name / GUID — join key
	SASAddress  string
	LinuxDevice string // correlated OS device, e.g. "sdao" (empty if unresolved)
	Protocol    string // SAS / SATA / ...
	DriveType   string // e.g. SAS_HDD, SATA_SSD
	State       string // controller-reported state, e.g. "Ready (RDY)"
	SizeBytes   int64
}

// Enclosure is a drive enclosure (shelf/backplane) and its slot range.
type Enclosure struct {
	Backend     string
	Controller  string
	Enclosure   string
	EnclosureID string
	NumSlots    int
	StartSlot   int
}

// Snapshot is a point-in-time view from a single backend.
type Snapshot struct {
	Drives     []Drive
	Enclosures []Enclosure
}

// Backend discovers drive topology from one source (an HBA tool, sysfs, …).
type Backend interface {
	// Name is the stable backend id used in metric labels and flags.
	Name() string
	// Available reports whether this backend can run here (tool present, etc.).
	Available() bool
	// Collect returns a fresh topology snapshot.
	Collect() (Snapshot, error)
}
