# Project Notes

## Related Repositories

- **Wyze Plugin**: `/Users/joshua.seidel/spatialnvr-wyze-plugin`
- **Reolink Plugin**: `/Users/joshua.seidel/spatialnvr-reolink-plugin`
- **Plugin Catalog**: `https://github.com/Spatial-NVR/plugin-catalog`

## Plugin Architecture

- Plugin manifests use `id` field (e.g., `wyze`, `reolink`) which may differ from directory names
- Plugins are installed to `data/plugins/{manifest-id}/`
- Manifest files can be either `manifest.yaml` or `manifest.json`

## Ports

- go2rtc RTSP: 8554
- go2rtc WebRTC: 8555
- wyze-bridge RTSP: 8564
- wyze-bridge WebRTC: 8561
- wyze-bridge HLS: 8562
- wyze-bridge Web UI: 5002
