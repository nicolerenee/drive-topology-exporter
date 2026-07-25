// Command drive-topology-exporter exposes physical drive topology (which
// enclosure slot each drive occupies, its identity, and its Linux device) as
// Prometheus metrics. It is zero-config and auto-detecting: layout/geometry and
// friendly names belong to the consuming inventory/UI, not here.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nicolerenee/drive-topology-exporter/internal/backends"
	"github.com/nicolerenee/drive-topology-exporter/internal/collector"
	"github.com/nicolerenee/drive-topology-exporter/internal/topo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var version = "dev" // set via -ldflags "-X main.version=..."

func main() {
	var (
		listenAddr   = flag.String("web.listen-address", ":9101", "Address to expose metrics on.")
		metricsPath  = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
		interval     = flag.Duration("collector.interval", 60*time.Second, "How often to refresh topology (backend tools can be slow, so scrapes are served from cache).")
		noSAS2IRCU   = flag.Bool("no-collector.sas2ircu", false, "Disable the sas2ircu backend.")
		sas2ircuPath = flag.String("sas2ircu.path", "", "Override the sas2ircu binary path (default: autodetect on PATH / common locations).")
		showVersion  = flag.Bool("version", false, "Print version and exit.")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("drive-topology-exporter", version)
		return
	}

	var enabled []topo.Backend
	if !*noSAS2IRCU {
		b := backends.NewSAS2IRCU(*sas2ircuPath)
		if b.Available() {
			enabled = append(enabled, b)
			log.Printf("backend sas2ircu: enabled")
		} else {
			log.Printf("backend sas2ircu: unavailable (sas2ircu not found or no controllers) — skipping")
		}
	}
	if len(enabled) == 0 {
		log.Fatal("no drive-topology backends available; nothing to export")
	}

	c := collector.New(enabled)
	c.Refresh() // prime the cache so the first scrape has data
	go func() {
		t := time.NewTicker(*interval)
		defer t.Stop()
		for range t.C {
			c.Refresh()
		}
	}()

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mux := http.NewServeMux()
	mux.Handle(*metricsPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "drive-topology-exporter %s\n\nMetrics: %s\n", version, *metricsPath)
	})

	log.Printf("drive-topology-exporter %s listening on %s%s (refresh %s)", version, *listenAddr, *metricsPath, *interval)
	log.Fatal(http.ListenAndServe(*listenAddr, mux))
}
