package linux

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiskInfo represents Linux disk information
type DiskInfo struct {
	DevicePath string // e.g., /dev/sda
	DeviceName string // e.g., sda
	Model      string
	Serial     string
	WWN        string
	Size       string
	Rotational bool
}

// GetDiskInfoBySerial finds Linux disk information by matching serial number
// It tries multiple methods: serial number, WWN, and by-id links
func GetDiskInfoBySerial(serial string) (*DiskInfo, error) {
	// Normalize the search serial
	searchSerial := strings.ToUpper(strings.TrimSpace(serial))

	// Search /sys/block for devices
	blockDir := "/sys/block"
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// Skip loop, ram, etc.
		if strings.HasPrefix(entry.Name(), "loop") ||
			strings.HasPrefix(entry.Name(), "ram") ||
			strings.HasPrefix(entry.Name(), "sr") ||
			strings.HasPrefix(entry.Name(), "fd") {
			continue
		}

		devicePath := filepath.Join(blockDir, entry.Name())
		deviceName := entry.Name()

		// Try to read serial number
		serialPath := filepath.Join(devicePath, "serial")
		serialBytes, err := os.ReadFile(serialPath)
		var deviceSerial string
		if err == nil {
			deviceSerial = strings.ToUpper(strings.TrimSpace(string(serialBytes)))
		}

		// Try to read WWN (World Wide Name) - SAS drives often use this
		wwnPath := filepath.Join(devicePath, "wwid")
		var deviceWWN string
		if wwnBytes, err := os.ReadFile(wwnPath); err == nil {
			deviceWWN = strings.TrimSpace(string(wwnBytes))
		}

		// Check if serial matches
		serialMatch := deviceSerial != "" && deviceSerial == searchSerial

		// Check if WWN contains the serial (some SAS drives encode serial in WWN)
		wwnMatch := false
		if deviceWWN != "" {
			// WWN format is often like "naa.5000cca273cde5d1" or "eui.5000cca273cde5d1"
			// The serial might be encoded in the hex part
			wwnUpper := strings.ToUpper(deviceWWN)
			wwnMatch = strings.Contains(wwnUpper, searchSerial)
		}

		// Also check /dev/disk/by-id for serial number matches
		byIdMatch := false
		byIdDir := "/dev/disk/by-id"
		if byIdEntries, err := os.ReadDir(byIdDir); err == nil {
			for _, byIdEntry := range byIdEntries {
				linkName := byIdEntry.Name()
				// Check if link name contains the serial
				if strings.Contains(strings.ToUpper(linkName), searchSerial) {
					// Resolve the symlink to see if it points to our device
					linkPath := filepath.Join(byIdDir, linkName)
					if target, err := os.Readlink(linkPath); err == nil {
						// Get the actual device name from the target
						// Target is usually like "../../sda"
						targetDevice := filepath.Base(target)
						if targetDevice == deviceName {
							byIdMatch = true
							break
						}
					}
				}
			}
		}

		// Match if any method succeeds
		if serialMatch || wwnMatch || byIdMatch {
			info := &DiskInfo{
				DevicePath: "/dev/" + deviceName,
				DeviceName: deviceName,
				Serial:     deviceSerial,
			}

			// Read model
			modelPath := filepath.Join(devicePath, "device", "model")
			if modelBytes, err := os.ReadFile(modelPath); err == nil {
				info.Model = strings.TrimSpace(string(modelBytes))
			}

			// Read WWN
			if deviceWWN != "" {
				info.WWN = deviceWWN
			}

			// Read size
			sizePath := filepath.Join(devicePath, "size")
			if sizeBytes, err := os.ReadFile(sizePath); err == nil {
				sizeStr := strings.TrimSpace(string(sizeBytes))
				// Size is in 512-byte sectors, convert to human readable
				info.Size = sizeStr // Will format later if needed
			}

			// Check if rotational
			rotationalPath := filepath.Join(devicePath, "queue", "rotational")
			if rotBytes, err := os.ReadFile(rotationalPath); err == nil {
				rotStr := strings.TrimSpace(string(rotBytes))
				info.Rotational = rotStr == "1"
			}

			return info, nil
		}
	}

	return nil, nil // Not found, but not an error
}

// GetDiskInfoByGUID finds Linux disk information by matching GUID
// GUIDs from sas2ircu are often in format like "5000cca273cde5d1" (hex without prefix)
func GetDiskInfoByGUID(guid string) (*DiskInfo, error) {
	// Normalize the GUID - remove common prefixes and normalize case
	searchGUID := strings.ToUpper(strings.TrimSpace(guid))
	// Remove "0x" prefix if present
	searchGUID = strings.TrimPrefix(searchGUID, "0X")

	// Search /sys/block for devices
	blockDir := "/sys/block"
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		// Skip loop, ram, etc.
		if strings.HasPrefix(entry.Name(), "loop") ||
			strings.HasPrefix(entry.Name(), "ram") ||
			strings.HasPrefix(entry.Name(), "sr") ||
			strings.HasPrefix(entry.Name(), "fd") {
			continue
		}

		devicePath := filepath.Join(blockDir, entry.Name())
		deviceName := entry.Name()

		// Read WWN (World Wide Name) - GUIDs are often encoded here
		wwnPath := filepath.Join(devicePath, "wwid")
		var deviceWWN string
		if wwnBytes, err := os.ReadFile(wwnPath); err == nil {
			deviceWWN = strings.TrimSpace(string(wwnBytes))
		}

		// WWN format is often like "naa.5000cca273cde5d1" or "eui.5000cca273cde5d1"
		// Extract the hex part and compare with GUID
		wwnMatch := false
		if deviceWWN != "" {
			wwnUpper := strings.ToUpper(deviceWWN)
			// Remove common prefixes
			wwnUpper = strings.TrimPrefix(wwnUpper, "NAA.")
			wwnUpper = strings.TrimPrefix(wwnUpper, "EUI.")
			// Check if the GUID matches the hex part of the WWN
			if strings.Contains(wwnUpper, searchGUID) || wwnUpper == searchGUID {
				wwnMatch = true
			}
		}

		// Also check /dev/disk/by-id for GUID matches
		byIdMatch := false
		byIdDir := "/dev/disk/by-id"
		if byIdEntries, err := os.ReadDir(byIdDir); err == nil {
			for _, byIdEntry := range byIdEntries {
				linkName := byIdEntry.Name()
				// Check if link name contains the GUID
				if strings.Contains(strings.ToUpper(linkName), searchGUID) {
					// Resolve the symlink to see if it points to our device
					linkPath := filepath.Join(byIdDir, linkName)
					if target, err := os.Readlink(linkPath); err == nil {
						// Get the actual device name from the target
						targetDevice := filepath.Base(target)
						if targetDevice == deviceName {
							byIdMatch = true
							break
						}
					}
				}
			}
		}

		// Match if any method succeeds
		if wwnMatch || byIdMatch {
			info := &DiskInfo{
				DevicePath: "/dev/" + deviceName,
				DeviceName: deviceName,
			}

			// Read serial number
			serialPath := filepath.Join(devicePath, "serial")
			if serialBytes, err := os.ReadFile(serialPath); err == nil {
				info.Serial = strings.TrimSpace(string(serialBytes))
			}

			// Read model
			modelPath := filepath.Join(devicePath, "device", "model")
			if modelBytes, err := os.ReadFile(modelPath); err == nil {
				info.Model = strings.TrimSpace(string(modelBytes))
			}

			// Read WWN
			if deviceWWN != "" {
				info.WWN = deviceWWN
			}

			// Read size
			sizePath := filepath.Join(devicePath, "size")
			if sizeBytes, err := os.ReadFile(sizePath); err == nil {
				sizeStr := strings.TrimSpace(string(sizeBytes))
				info.Size = sizeStr
			}

			// Check if rotational
			rotationalPath := filepath.Join(devicePath, "queue", "rotational")
			if rotBytes, err := os.ReadFile(rotationalPath); err == nil {
				rotStr := strings.TrimSpace(string(rotBytes))
				info.Rotational = rotStr == "1"
			}

			return info, nil
		}
	}

	return nil, nil // Not found, but not an error
}

// ATADevice represents an ATA device found by controller path
type ATADevice struct {
	Port       int    // ATA port number (1, 2, etc.)
	DevicePath string // e.g., /dev/sda
	DeviceName string // e.g., sda
	DiskInfo   *DiskInfo
}

// NVMeDevice represents an NVMe device found by controller path
type NVMeDevice struct {
	Port       int    // NVMe port number (1, 2, etc.)
	DevicePath string // e.g., /dev/nvme0n1
	DeviceName string // e.g., nvme0n1
	DiskInfo   *DiskInfo
}

// GetATADevicesByControllerPaths discovers ATA devices connected to one or more controllers
// controllerPaths can be a single path like "pci-0000:00:1f.2" or comma-separated paths like "pci-0000:00:13.0,pci-0000:00:14.0"
// Returns a map of port number to ATADevice, with ports numbered sequentially across all controllers
func GetATADevicesByControllerPaths(controllerPaths string) (map[int]*ATADevice, error) {
	// Split by comma to support multiple controller paths
	paths := strings.Split(controllerPaths, ",")
	for i := range paths {
		paths[i] = strings.TrimSpace(paths[i])
	}

	result := make(map[int]*ATADevice)
	byPathDir := "/dev/disk/by-path"

	entries, err := os.ReadDir(byPathDir)
	if err != nil {
		return nil, err
	}

	// Track all devices found, grouped by controller path
	devicesByController := make(map[string][]*ATADevice)

	for _, controllerPath := range paths {
		// Pattern: pci-0000:00:1f.2-ata-1, pci-0000:00:1f.2-ata-2, etc.
		prefix := controllerPath + "-ata-"
		controllerDevices := make([]*ATADevice, 0)

		for _, entry := range entries {
			linkName := entry.Name()
			if !strings.HasPrefix(linkName, prefix) {
				continue
			}

			// Extract port number from link name
			// Format: pci-0000:00:1f.2-ata-1 or pci-0000:00:1f.2-ata-1.0
			// We want the number after "ata-"
			portStr := strings.TrimPrefix(linkName, prefix)
			// Remove any suffix like ".0" or "-part1"
			if dotIdx := strings.Index(portStr, "."); dotIdx != -1 {
				portStr = portStr[:dotIdx]
			}
			if dashIdx := strings.Index(portStr, "-"); dashIdx != -1 {
				portStr = portStr[:dashIdx]
			}

			// Parse port number
			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
				continue
			}

			// Skip if we already have this port for this controller (prefer non-partition links)
			found := false
			for _, existing := range controllerDevices {
				if existing.Port == port {
					// If current link is a partition link, skip it
					if strings.Contains(linkName, "-part") || strings.Contains(linkName, ".") {
						found = true
						break
					}
					// If existing is a partition link, replace it
					if strings.Contains(existing.DevicePath, "-part") {
						// Remove old one and continue to add new one
						for i, d := range controllerDevices {
							if d.Port == port {
								controllerDevices = append(controllerDevices[:i], controllerDevices[i+1:]...)
								break
							}
						}
						break
					}
					found = true
					break
				}
			}
			if found && (strings.Contains(linkName, "-part") || strings.Contains(linkName, ".")) {
				continue
			}

			// Resolve the symlink to get the actual device
			linkPath := filepath.Join(byPathDir, linkName)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}

			// Skip partition links (we want the whole disk)
			if strings.Contains(linkName, "-part") {
				continue
			}

			// Get device name from target (usually "../../sda")
			deviceName := filepath.Base(target)
			devicePath := "/dev/" + deviceName

			// Get disk info for this device
			diskInfo, err := getDiskInfoByDevice(deviceName)
			if err != nil {
				continue
			}

			controllerDevices = append(controllerDevices, &ATADevice{
				Port:       port,
				DevicePath: devicePath,
				DeviceName: deviceName,
				DiskInfo:   diskInfo,
			})
		}

		// Sort devices by port number for this controller
		for i := 0; i < len(controllerDevices)-1; i++ {
			for j := i + 1; j < len(controllerDevices); j++ {
				if controllerDevices[i].Port > controllerDevices[j].Port {
					controllerDevices[i], controllerDevices[j] = controllerDevices[j], controllerDevices[i]
				}
			}
		}

		devicesByController[controllerPath] = controllerDevices
	}

	// Now assign sequential slot numbers across all controllers
	slot := 1
	for _, controllerPath := range paths {
		controllerDevices := devicesByController[controllerPath]
		for _, device := range controllerDevices {
			// Create a new device with sequential slot number
			result[slot] = &ATADevice{
				Port:       slot, // Use sequential slot number
				DevicePath: device.DevicePath,
				DeviceName: device.DeviceName,
				DiskInfo:   device.DiskInfo,
			}
			slot++
		}
	}

	return result, nil
}

// GetATADevicesByControllerPath discovers ATA devices connected to a specific controller
// controllerPath is like "pci-0000:00:1f.2"
// Returns a map of port number to ATADevice
// This is a convenience wrapper around GetATADevicesByControllerPaths for backward compatibility
func GetATADevicesByControllerPath(controllerPath string) (map[int]*ATADevice, error) {
	return GetATADevicesByControllerPaths(controllerPath)
}

// ControllerPortMapping represents a controller path and its port numbers
type ControllerPortMapping struct {
	ControllerPath string
	Ports         []int
}

// GetNVMeDevicesByControllerPaths discovers NVMe devices connected to one or more controllers
// controllerPaths can be a single path like "pci-0000:03:00.0" or comma-separated paths
// Returns a map of port number to NVMeDevice, with ports numbered sequentially across all controllers
func GetNVMeDevicesByControllerPaths(controllerPaths string) (map[int]*NVMeDevice, error) {
	// Split by comma to support multiple controller paths
	paths := strings.Split(controllerPaths, ",")
	for i := range paths {
		paths[i] = strings.TrimSpace(paths[i])
	}

	result := make(map[int]*NVMeDevice)
	byPathDir := "/dev/disk/by-path"

	entries, err := os.ReadDir(byPathDir)
	if err != nil {
		return nil, err
	}

	// Track all devices found, grouped by controller path
	devicesByController := make(map[string][]*NVMeDevice)

	for _, controllerPath := range paths {
		// Pattern: pci-0000:03:00.0-nvme-1, pci-0000:03:00.0-nvme-2, etc.
		prefix := controllerPath + "-nvme-"
		controllerDevices := make([]*NVMeDevice, 0)

		for _, entry := range entries {
			linkName := entry.Name()
			if !strings.HasPrefix(linkName, prefix) {
				continue
			}

			// Extract port number from link name
			// Format: pci-0000:03:00.0-nvme-1 or pci-0000:03:00.0-nvme-1.0
			// We want the number after "nvme-"
			portStr := strings.TrimPrefix(linkName, prefix)
			// Remove any suffix like ".0" or "-part1"
			if dotIdx := strings.Index(portStr, "."); dotIdx != -1 {
				portStr = portStr[:dotIdx]
			}
			if dashIdx := strings.Index(portStr, "-"); dashIdx != -1 {
				portStr = portStr[:dashIdx]
			}

			// Parse port number
			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
				continue
			}

			// Skip if we already have this port for this controller (prefer non-partition links)
			found := false
			for _, existing := range controllerDevices {
				if existing.Port == port {
					// If current link is a partition link, skip it
					if strings.Contains(linkName, "-part") || strings.Contains(linkName, ".") {
						found = true
						break
					}
					// If existing is a partition link, replace it
					if strings.Contains(existing.DevicePath, "-part") {
						// Remove old one and continue to add new one
						for i, d := range controllerDevices {
							if d.Port == port {
								controllerDevices = append(controllerDevices[:i], controllerDevices[i+1:]...)
								break
							}
						}
						break
					}
					found = true
					break
				}
			}
			if found && (strings.Contains(linkName, "-part") || strings.Contains(linkName, ".")) {
				continue
			}

			// Resolve the symlink to get the actual device
			linkPath := filepath.Join(byPathDir, linkName)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}

			// Skip partition links (we want the whole disk)
			if strings.Contains(linkName, "-part") {
				continue
			}

			// Get device name from target (usually "../../nvme0n1")
			deviceName := filepath.Base(target)
			devicePath := "/dev/" + deviceName

			// Get disk info for this device
			diskInfo, err := getDiskInfoByDevice(deviceName)
			if err != nil {
				continue
			}

			controllerDevices = append(controllerDevices, &NVMeDevice{
				Port:       port,
				DevicePath: devicePath,
				DeviceName: deviceName,
				DiskInfo:   diskInfo,
			})
		}

		// Sort devices by port number for this controller
		for i := 1; i < len(controllerDevices); i++ {
			key := controllerDevices[i]
			j := i - 1
			for j >= 0 && controllerDevices[j].Port > key.Port {
				controllerDevices[j+1] = controllerDevices[j]
				j--
			}
			controllerDevices[j+1] = key
		}

		devicesByController[controllerPath] = controllerDevices
	}

	// Now assign sequential slot numbers across all controllers
	slot := 1
	for _, controllerPath := range paths {
		controllerDevices := devicesByController[controllerPath]
		for _, device := range controllerDevices {
			// Create a new device with sequential slot number
			result[slot] = &NVMeDevice{
				Port:       slot, // Use sequential slot number
				DevicePath: device.DevicePath,
				DeviceName: device.DeviceName,
				DiskInfo:   device.DiskInfo,
			}
			slot++
		}
	}

	return result, nil
}

// GetNVMeDevicesByControllerPath discovers NVMe devices connected to a specific controller
// controllerPath is like "pci-0000:03:00.0"
// Returns a map of port number to NVMeDevice
// This is a convenience wrapper around GetNVMeDevicesByControllerPaths for backward compatibility
func GetNVMeDevicesByControllerPath(controllerPath string) (map[int]*NVMeDevice, error) {
	return GetNVMeDevicesByControllerPaths(controllerPath)
}

// GetNVMeDevicesByExplicitPorts discovers NVMe devices using explicit controller-to-port mappings
// mappings is a slice of controller paths and their port numbers
// Returns a map of slot number to NVMeDevice, with slots numbered sequentially across all mappings
// Missing drives will have nil entries in the map
func GetNVMeDevicesByExplicitPorts(mappings []ControllerPortMapping) (map[int]*NVMeDevice, error) {
	result := make(map[int]*NVMeDevice)
	byPathDir := "/dev/disk/by-path"

	entries, err := os.ReadDir(byPathDir)
	if err != nil {
		return nil, err
	}

	// Build a map of (controllerPath, port) -> device for quick lookup
	deviceMap := make(map[string]map[int]*NVMeDevice) // map[controllerPath]map[port]*NVMeDevice

	// First, discover all devices from all controllers
	for _, mapping := range mappings {
		controllerPath := mapping.ControllerPath
		prefix := controllerPath + "-nvme-"
		controllerDevices := make(map[int]*NVMeDevice)

		for _, entry := range entries {
			linkName := entry.Name()
			if !strings.HasPrefix(linkName, prefix) {
				continue
			}

			// Extract port number from link name
			portStr := strings.TrimPrefix(linkName, prefix)
			// Remove any suffix like ".0" or "-part1"
			if dotIdx := strings.Index(portStr, "."); dotIdx != -1 {
				portStr = portStr[:dotIdx]
			}
			if dashIdx := strings.Index(portStr, "-"); dashIdx != -1 {
				portStr = portStr[:dashIdx]
			}

			// Parse port number
			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
				continue
			}

			// Skip partition links (we want the whole disk)
			if strings.Contains(linkName, "-part") {
				continue
			}

			// Skip if we already have this port for this controller (prefer non-partition links)
			if existing, exists := controllerDevices[port]; exists {
				// If current link is a partition link, skip it
				if strings.Contains(linkName, "-part") || strings.Contains(linkName, ".") {
					continue
				}
				// If existing is a partition link, replace it
				if strings.Contains(existing.DevicePath, "-part") {
					// Will be replaced below
				} else {
					continue
				}
			}

			// Resolve the symlink to get the actual device
			linkPath := filepath.Join(byPathDir, linkName)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}

			// Get device name from target (usually "../../nvme0n1")
			deviceName := filepath.Base(target)
			devicePath := "/dev/" + deviceName

			// Get disk info for this device
			diskInfo, err := getDiskInfoByDevice(deviceName)
			if err != nil {
				continue
			}

			controllerDevices[port] = &NVMeDevice{
				Port:       port,
				DevicePath: devicePath,
				DeviceName: deviceName,
				DiskInfo:   diskInfo,
			}
		}

		deviceMap[controllerPath] = controllerDevices
	}

	// Now assign sequential slot numbers based on explicit mappings
	slot := 1
	for _, mapping := range mappings {
		controllerPath := mapping.ControllerPath
		controllerDevices := deviceMap[controllerPath]

		// For each port in the mapping, assign it to a slot
		for _, port := range mapping.Ports {
			if device, found := controllerDevices[port]; found {
				// Device exists, assign it to this slot
				result[slot] = &NVMeDevice{
					Port:       slot, // Use sequential slot number
					DevicePath: device.DevicePath,
					DeviceName: device.DeviceName,
					DiskInfo:   device.DiskInfo,
				}
			}
			// If device not found, slot will be nil (missing drive)
			// This is handled by the caller when creating DriveInfo
			slot++
		}
	}

	return result, nil
}

// GetATADevicesByExplicitPorts discovers ATA devices using explicit controller-to-port mappings
// mappings is a slice of controller paths and their port numbers
// Returns a map of slot number to ATADevice, with slots numbered sequentially across all mappings
// Missing drives will have nil entries in the map
func GetATADevicesByExplicitPorts(mappings []ControllerPortMapping) (map[int]*ATADevice, error) {
	result := make(map[int]*ATADevice)
	byPathDir := "/dev/disk/by-path"

	entries, err := os.ReadDir(byPathDir)
	if err != nil {
		return nil, err
	}

	// Build a map of (controllerPath, port) -> device for quick lookup
	deviceMap := make(map[string]map[int]*ATADevice) // map[controllerPath]map[port]*ATADevice

	// First, discover all devices from all controllers
	for _, mapping := range mappings {
		controllerPath := mapping.ControllerPath
		prefix := controllerPath + "-ata-"
		controllerDevices := make(map[int]*ATADevice)

		for _, entry := range entries {
			linkName := entry.Name()
			if !strings.HasPrefix(linkName, prefix) {
				continue
			}

			// Extract port number from link name
			portStr := strings.TrimPrefix(linkName, prefix)
			// Remove any suffix like ".0" or "-part1"
			if dotIdx := strings.Index(portStr, "."); dotIdx != -1 {
				portStr = portStr[:dotIdx]
			}
			if dashIdx := strings.Index(portStr, "-"); dashIdx != -1 {
				portStr = portStr[:dashIdx]
			}

			// Parse port number
			var port int
			if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
				continue
			}

			// Skip partition links (we want the whole disk)
			if strings.Contains(linkName, "-part") {
				continue
			}

			// Skip if we already have this port (prefer non-partition links)
			if existing, exists := controllerDevices[port]; exists {
				// If current link is a partition link, skip it
				if strings.Contains(linkName, "-part") || strings.Contains(linkName, ".") {
					continue
				}
				// If existing is a partition link, replace it
				if strings.Contains(existing.DevicePath, "-part") {
					// Will be replaced below
				} else {
					continue
				}
			}

			// Resolve the symlink to get the actual device
			linkPath := filepath.Join(byPathDir, linkName)
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}

			// Get device name from target (usually "../../sda")
			deviceName := filepath.Base(target)
			devicePath := "/dev/" + deviceName

			// Get disk info for this device
			diskInfo, err := getDiskInfoByDevice(deviceName)
			if err != nil {
				continue
			}

			controllerDevices[port] = &ATADevice{
				Port:       port,
				DevicePath: devicePath,
				DeviceName: deviceName,
				DiskInfo:   diskInfo,
			}
		}

		deviceMap[controllerPath] = controllerDevices
	}

	// Now assign sequential slot numbers based on explicit mappings
	slot := 1
	for _, mapping := range mappings {
		controllerPath := mapping.ControllerPath
		controllerDevices := deviceMap[controllerPath]

		// For each port in the mapping, assign it to a slot
		for _, port := range mapping.Ports {
			if device, found := controllerDevices[port]; found {
				// Device exists, assign it to this slot
				result[slot] = &ATADevice{
					Port:       slot, // Use sequential slot number
					DevicePath: device.DevicePath,
					DeviceName: device.DeviceName,
					DiskInfo:   device.DiskInfo,
				}
			}
			// If device not found, slot will be nil (missing drive)
			// This is handled by the caller when creating DriveInfo
			slot++
		}
	}

	return result, nil
}

// getDiskInfoByDevice gets disk information for a specific device name
func getDiskInfoByDevice(deviceName string) (*DiskInfo, error) {
	blockDir := "/sys/block"
	devicePath := filepath.Join(blockDir, deviceName)

	// Check if device exists
	if _, err := os.Stat(devicePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("device %s does not exist", deviceName)
	}

	info := &DiskInfo{
		DevicePath: "/dev/" + deviceName,
		DeviceName: deviceName,
	}

	// Read serial number
	serialPath := filepath.Join(devicePath, "serial")
	if serialBytes, err := os.ReadFile(serialPath); err == nil {
		info.Serial = strings.TrimSpace(string(serialBytes))
	}

	// Read model
	modelPath := filepath.Join(devicePath, "device", "model")
	if modelBytes, err := os.ReadFile(modelPath); err == nil {
		info.Model = strings.TrimSpace(string(modelBytes))
	}

	// Read WWN
	wwnPath := filepath.Join(devicePath, "wwid")
	if wwnBytes, err := os.ReadFile(wwnPath); err == nil {
		info.WWN = strings.TrimSpace(string(wwnBytes))
	}

	// Read size
	sizePath := filepath.Join(devicePath, "size")
	if sizeBytes, err := os.ReadFile(sizePath); err == nil {
		sizeStr := strings.TrimSpace(string(sizeBytes))
		info.Size = sizeStr
	}

	// Check if rotational
	rotationalPath := filepath.Join(devicePath, "queue", "rotational")
	if rotBytes, err := os.ReadFile(rotationalPath); err == nil {
		rotStr := strings.TrimSpace(string(rotBytes))
		info.Rotational = rotStr == "1"
	}

	return info, nil
}
