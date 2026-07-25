// Package collector turns backend snapshots into Prometheus metrics.
package collector

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/nicolerenee/drive-topology-exporter/internal/topo"
	"github.com/prometheus/client_golang/prometheus"
)

const ns = "drive_topology"

var (
	// info is the join-key metric: one series per physical drive, value 1.
	infoDesc = prometheus.NewDesc(ns+"_info",
		"Physical drive topology and identity (value is always 1).",
		[]string{"backend", "controller", "enclosure", "enclosure_id", "slot",
			"serial", "model", "wwn", "sas_address", "linux_device", "protocol", "drive_type", "state"}, nil)

	presentDesc = prometheus.NewDesc(ns+"_slot_present",
		"Whether an enclosure bay is populated (1) or empty (0).",
		[]string{"backend", "controller", "enclosure", "enclosure_id", "slot"}, nil)

	sizeDesc = prometheus.NewDesc(ns+"_drive_size_bytes",
		"Drive capacity in bytes as reported by the backend.",
		[]string{"backend", "enclosure_id", "slot", "serial"}, nil)

	enclosureSlotsDesc = prometheus.NewDesc(ns+"_enclosure_slots",
		"Number of slots in an enclosure.",
		[]string{"backend", "controller", "enclosure", "enclosure_id"}, nil)

	backendUpDesc = prometheus.NewDesc(ns+"_backend_up",
		"Whether the last collection for a backend succeeded (1) or failed (0).",
		[]string{"backend"}, nil)

	scrapeDurationDesc = prometheus.NewDesc(ns+"_scrape_duration_seconds",
		"Duration of the last collection for a backend.",
		[]string{"backend"}, nil)
)

type entry struct {
	snap topo.Snapshot
	up   float64
	dur  float64
}

// Collector runs a set of backends and exposes their cached topology.
type Collector struct {
	backends []topo.Backend
	mu       sync.RWMutex
	cache    map[string]entry
}

func New(backends []topo.Backend) *Collector {
	return &Collector{backends: backends, cache: make(map[string]entry)}
}

// Refresh collects every backend once and updates the cache. sas2ircu DISPLAY is
// slow, so this runs on an interval rather than on every scrape.
func (c *Collector) Refresh() {
	for _, b := range c.backends {
		start := time.Now()
		snap, err := b.Collect()
		e := entry{snap: snap, up: 1, dur: time.Since(start).Seconds()}
		if err != nil {
			e.up = 0
			log.Printf("backend %s: collect failed: %v", b.Name(), err)
			// keep the previous snapshot on failure
			c.mu.RLock()
			prev, ok := c.cache[b.Name()]
			c.mu.RUnlock()
			if ok {
				e.snap = prev.snap
			}
		}
		c.mu.Lock()
		c.cache[b.Name()] = e
		c.mu.Unlock()
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- infoDesc
	ch <- presentDesc
	ch <- sizeDesc
	ch <- enclosureSlotsDesc
	ch <- backendUpDesc
	ch <- scrapeDurationDesc
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, e := range c.cache {
		ch <- prometheus.MustNewConstMetric(backendUpDesc, prometheus.GaugeValue, e.up, name)
		ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, e.dur, name)

		// occupied[enclosure_id][slot] = true, to emit slot_present for empty bays.
		occupied := map[string]map[int]bool{}
		for _, d := range e.snap.Drives {
			slot := strconv.Itoa(d.Slot)
			ch <- prometheus.MustNewConstMetric(infoDesc, prometheus.GaugeValue, 1,
				d.Backend, d.Controller, d.Enclosure, d.EnclosureID, slot,
				d.Serial, d.Model, d.WWN, d.SASAddress, d.LinuxDevice, d.Protocol, d.DriveType, d.State)
			if d.SizeBytes > 0 {
				ch <- prometheus.MustNewConstMetric(sizeDesc, prometheus.GaugeValue, float64(d.SizeBytes),
					d.Backend, d.EnclosureID, slot, d.Serial)
			}
			if occupied[d.EnclosureID] == nil {
				occupied[d.EnclosureID] = map[int]bool{}
			}
			occupied[d.EnclosureID][d.Slot] = true
		}

		for _, en := range e.snap.Enclosures {
			ch <- prometheus.MustNewConstMetric(enclosureSlotsDesc, prometheus.GaugeValue, float64(en.NumSlots),
				en.Backend, en.Controller, en.Enclosure, en.EnclosureID)
			for slot := en.StartSlot; slot < en.StartSlot+en.NumSlots; slot++ {
				present := 0.0
				if occupied[en.EnclosureID][slot] {
					present = 1.0
				}
				ch <- prometheus.MustNewConstMetric(presentDesc, prometheus.GaugeValue, present,
					en.Backend, en.Controller, en.Enclosure, en.EnclosureID, strconv.Itoa(slot))
			}
		}
	}
}
