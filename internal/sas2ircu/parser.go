package sas2ircu

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Controller represents a SAS controller
type Controller struct {
	Index       int
	Type        string
	VendorID    string
	DeviceID    string
	PCIPath     string
	SubsysVenID string
	SubsysDevID string
}

// PhysicalDevice represents a physical device (drive) from sas2ircu
type PhysicalDevice struct {
	EnclosureNum    int
	SlotNum         int
	SASAddress      string
	State           string
	SizeMB          int64
	SizeSectors     int64
	Manufacturer    string
	ModelNumber     string
	FirmwareRev     string
	SerialNo        string
	GUID            string
	Protocol        string
	DriveType       string
	DeviceType      string
}

// EnclosureInfo represents enclosure information
type EnclosureInfo struct {
	EnclosureNum int
	LogicalID    string
	NumSlots     int
	StartSlot    int
}

// ControllerDetails represents detailed controller information
type ControllerDetails struct {
	ControllerType           string
	BIOSVersion              string
	FirmwareVersion          string
	ChannelDescription       string
	InitiatorID              int
	MaxPhysicalDevices       int
	ConcurrentCommands       int
	Slot                     int
	Segment                  int
	Bus                      int
	Device                   int
	Function                 int
	RAIDSupport              bool
	PhysicalDevices          []PhysicalDevice
	Enclosures               []EnclosureInfo
}

// Parser handles parsing of sas2ircu output
type Parser struct {
	sas2ircuPath string
}

// NewParser creates a new sas2ircu parser
func NewParser() *Parser {
	// Try to find sas2ircu in PATH or common locations
	path := findSas2ircu()
	return &Parser{
		sas2ircuPath: path,
	}
}

// NewParserWithPath creates a parser using an explicit sas2ircu binary path.
func NewParserWithPath(path string) *Parser {
	if path == "" {
		return NewParser()
	}
	return &Parser{sas2ircuPath: path}
}

// findSas2ircu tries to locate the sas2ircu binary
func findSas2ircu() string {
	// First try to find it in PATH
	if path, err := exec.LookPath("sas2ircu"); err == nil {
		return path
	}

	// Try common locations
	commonPaths := []string{
		"/usr/sbin/sas2ircu",
		"/usr/bin/sas2ircu",
		"/sbin/sas2ircu",
		"/bin/sas2ircu",
		"/opt/lsi/sas2ircu",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Fallback to just "sas2ircu" and let exec.Command handle the error
	return "sas2ircu"
}

// ListControllers executes "sas2ircu LIST" and parses the output
func (p *Parser) ListControllers() ([]Controller, error) {
	cmd := exec.Command(p.sas2ircuPath, "LIST")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute sas2ircu LIST: %w (output: %s)", err, string(output))
	}

	controllers, parseErr := p.parseListOutput(string(output))
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse sas2ircu LIST output: %w", parseErr)
	}

	return controllers, nil
}

// DisplayController executes "sas2ircu <index> DISPLAY" and parses the output
func (p *Parser) DisplayController(index int) (*ControllerDetails, error) {
	cmd := exec.Command(p.sas2ircuPath, strconv.Itoa(index), "DISPLAY")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to execute sas2ircu DISPLAY: %w (output: %s)", err, string(output))
	}

	details, parseErr := p.parseDisplayOutput(string(output))
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse sas2ircu DISPLAY output: %w", parseErr)
	}

	return details, nil
}

// parseListOutput parses the output of "sas2ircu LIST"
func (p *Parser) parseListOutput(output string) ([]Controller, error) {
	var controllers []Controller

	// Pattern to match controller lines:
	//   0     SAS2008     1000h    72h   00h:05h:00h:00h      1000h   3020h
	// The pattern needs to be more flexible to handle varying whitespace
	controllerPattern := regexp.MustCompile(`^\s+(\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// Skip header lines
		if strings.Contains(line, "Adapter") || strings.Contains(line, "Index") ||
		   strings.Contains(line, "-----") || strings.Contains(line, "LSI Corporation") ||
		   strings.Contains(line, "Version") || strings.Contains(line, "Copyright") ||
		   strings.Contains(line, "SAS2IRCU") || strings.TrimSpace(line) == "" {
			continue
		}

		matches := controllerPattern.FindStringSubmatch(line)
		if len(matches) == 8 {
			index, err := strconv.Atoi(matches[1])
			if err != nil {
				continue
			}

			controllers = append(controllers, Controller{
				Index:       index,
				Type:        matches[2],
				VendorID:    matches[3],
				DeviceID:    matches[4],
				PCIPath:     matches[5],
				SubsysVenID: matches[6],
				SubsysDevID: matches[7],
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning output: %w", err)
	}

	if len(controllers) == 0 {
		// Return a helpful error with a sample of the output
		lines := strings.Split(output, "\n")
		sample := ""
		if len(lines) > 0 {
			if len(lines) > 10 {
				sample = strings.Join(lines[:10], "\n")
			} else {
				sample = strings.Join(lines, "\n")
			}
		}
		return nil, fmt.Errorf("no controllers found in sas2ircu LIST output. Sample output:\n%s", sample)
	}

	return controllers, nil
}

// parseDisplayOutput parses the output of "sas2ircu <index> DISPLAY"
func (p *Parser) parseDisplayOutput(output string) (*ControllerDetails, error) {
	details := &ControllerDetails{
		PhysicalDevices: []PhysicalDevice{},
		Enclosures:      []EnclosureInfo{},
	}

	scanner := bufio.NewScanner(strings.NewReader(output))

	var currentSection string
	var currentDevice *PhysicalDevice

	for scanner.Scan() {
		line := scanner.Text()

		// Detect sections
		if strings.Contains(line, "Controller information") {
			currentSection = "controller"
			continue
		}
		if strings.Contains(line, "Physical device information") {
			currentSection = "physical"
			continue
		}
		if strings.Contains(line, "Enclosure information") {
			currentSection = "enclosure"
			continue
		}

		// Parse controller information
		if currentSection == "controller" {
			p.parseControllerLine(line, details)
		}

		// Parse physical device information
		if currentSection == "physical" {
			if idx := strings.Index(line, "Device is a"); idx != -1 {
				// Start of a new device; capture the type from the marker line
				// ("Device is a Hard disk" / "Enclosure services device").
				if currentDevice != nil {
					details.PhysicalDevices = append(details.PhysicalDevices, *currentDevice)
				}
				currentDevice = &PhysicalDevice{
					DeviceType: strings.TrimSpace(line[idx+len("Device is a"):]),
				}
				continue
			}
			if currentDevice != nil {
				p.parseDeviceLine(line, currentDevice)
			}
		}

		// Parse enclosure information
		if currentSection == "enclosure" {
			p.parseEnclosureLine(line, details)
		}
	}

	// Don't forget the last device
	if currentDevice != nil {
		details.PhysicalDevices = append(details.PhysicalDevices, *currentDevice)
	}

	return details, scanner.Err()
}

func (p *Parser) parseControllerLine(line string, details *ControllerDetails) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	switch key {
	case "Controller type":
		details.ControllerType = value
	case "BIOS version":
		details.BIOSVersion = value
	case "Firmware version":
		details.FirmwareVersion = value
	case "Channel description":
		details.ChannelDescription = value
	case "Initiator ID":
		if id, err := strconv.Atoi(value); err == nil {
			details.InitiatorID = id
		}
	case "Maximum physical devices":
		if max, err := strconv.Atoi(value); err == nil {
			details.MaxPhysicalDevices = max
		}
	case "Concurrent commands supported":
		if cmds, err := strconv.Atoi(value); err == nil {
			details.ConcurrentCommands = cmds
		}
	case "Slot":
		if slot, err := strconv.Atoi(value); err == nil {
			details.Slot = slot
		}
	case "Segment":
		if seg, err := strconv.Atoi(value); err == nil {
			details.Segment = seg
		}
	case "Bus":
		if bus, err := strconv.Atoi(value); err == nil {
			details.Bus = bus
		}
	case "Device":
		if dev, err := strconv.Atoi(value); err == nil {
			details.Device = dev
		}
	case "Function":
		if fn, err := strconv.Atoi(value); err == nil {
			details.Function = fn
		}
	case "RAID Support":
		details.RAIDSupport = strings.ToLower(value) == "yes"
	}
}

func (p *Parser) parseDeviceLine(line string, device *PhysicalDevice) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	switch key {
	case "Enclosure #":
		if enc, err := strconv.Atoi(value); err == nil {
			device.EnclosureNum = enc
		}
	case "Slot #":
		if slot, err := strconv.Atoi(value); err == nil {
			device.SlotNum = slot
		}
	case "SAS Address":
		device.SASAddress = value
	case "State":
		device.State = value
	case "Size (in MB)/(in sectors)":
		// Format: "9537535/19532873727"
		sizeParts := strings.Split(value, "/")
		if len(sizeParts) == 2 {
			if mb, err := strconv.ParseInt(strings.TrimSpace(sizeParts[0]), 10, 64); err == nil {
				device.SizeMB = mb
			}
			if sectors, err := strconv.ParseInt(strings.TrimSpace(sizeParts[1]), 10, 64); err == nil {
				device.SizeSectors = sectors
			}
		}
	case "Manufacturer":
		device.Manufacturer = value
	case "Model Number":
		device.ModelNumber = value
	case "Firmware Revision":
		device.FirmwareRev = value
	case "Serial No":
		device.SerialNo = value
	case "GUID":
		device.GUID = value
	case "Protocol":
		device.Protocol = value
	case "Drive Type":
		device.DriveType = value
	case "Device Type":
		device.DeviceType = value
	}
}

func (p *Parser) parseEnclosureLine(line string, details *ControllerDetails) {
	// Format: "  Enclosure#                              : 2"
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if strings.Contains(key, "Enclosure#") {
		enc := EnclosureInfo{}
		if encNum, err := strconv.Atoi(value); err == nil {
			enc.EnclosureNum = encNum
		}
		details.Enclosures = append(details.Enclosures, enc)
	} else if len(details.Enclosures) > 0 {
		// Update the last enclosure
		enc := &details.Enclosures[len(details.Enclosures)-1]
		switch key {
		case "Logical ID":
			enc.LogicalID = value
		case "Numslots":
			if slots, err := strconv.Atoi(value); err == nil {
				enc.NumSlots = slots
			}
		case "StartSlot":
			if start, err := strconv.Atoi(value); err == nil {
				enc.StartSlot = start
			}
		}
	}
}
