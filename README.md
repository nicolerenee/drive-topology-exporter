# drive-topology-exporter

A small, zero-config Prometheus exporter for **physical drive topology** — which
enclosure slot each drive occupies, its identity (serial / model / WWN), and the
Linux device it maps to. It's the glue that lets you join SMART, ZFS, and node
metrics *by physical location*.

It deliberately does **not** re-export SMART, ZFS, or node metrics — use
`smartctl_exporter`, a zfs/zpool exporter, and `node_exporter` for those. This
exporter only emits the topology + identity, keyed so the others join cleanly.

## What it emits

| metric | labels | meaning |
| --- | --- | --- |
| `drive_topology_info` | `backend, controller, enclosure, enclosure_id, slot, serial, model, wwn, sas_address, linux_device, protocol, drive_type, state` | one series per drive, value `1` (join key) |
| `drive_topology_slot_present` | `backend, controller, enclosure, enclosure_id, slot` | bay populated (`1`) or empty (`0`) |
| `drive_topology_drive_size_bytes` | `backend, enclosure_id, slot, serial` | capacity |
| `drive_topology_enclosure_slots` | `backend, controller, enclosure, enclosure_id` | slot count |
| `drive_topology_backend_up` | `backend` | last collection ok |
| `drive_topology_scrape_duration_seconds` | `backend` | collection time |

`serial` / `wwn` / `linux_device` are the join keys:
`drive_topology_info * on(serial) group_left(...) smartctl_device` etc.

`enclosure_id` is the **stable** SES logical ID — map it to a friendly name /
row-col geometry / rack-U in *your* inventory. The exporter emits only detectable
facts; layout is not its job.

## Design

- **Zero config.** Auto-detects backends; no config file. Friendly names,
  enclosure geometry, and rack positions live in your inventory/UI, never here.
- **Pluggable backends.** `sas2ircu` today; `sas3ircu`, `sysfs`, and `nvme` are
  just additional backends behind one interface.
- **Cached.** Backend tools (e.g. `sas2ircu DISPLAY`) are slow, so topology is
  refreshed on an interval and scrapes are served from cache.

## Flags

```
--web.listen-address=:9101
--web.telemetry-path=/metrics
--collector.interval=60s
--no-collector.sas2ircu          # disable a backend
--sas2ircu.path=/usr/local/bin/sas2ircu   # override autodetection
```

## Running

The exporter shells out to backend tools (`sas2ircu`) and reads `/sys/block` +
`/dev/disk/by-id` to correlate OS devices, so it needs host device access. The
published image (`ghcr.io/nicolerenee/drive-topology-exporter`) is a static
binary only — **`sas2ircu` is proprietary and is not bundled**; provide it from
the host.

Example (Docker, on a host that has `sas2ircu`):

```yaml
services:
  drive-topology-exporter:
    image: ghcr.io/nicolerenee/drive-topology-exporter:main
    privileged: true            # HBA + raw device access
    ports: ["9101:9101"]
    volumes:
      - /usr/local/bin/sas2ircu:/usr/local/bin/sas2ircu:ro
      - /dev:/dev
      - /sys:/sys
```

## License

MIT — see [LICENSE](LICENSE).
