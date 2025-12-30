# Project Notes

## CRITICAL RULES - DO NOT VIOLATE

1. **NEVER question the user's container version.** When the user says they are running the latest container, they ARE running the latest container. Do not suggest they need to pull, update, or recreate containers. Do not compare image digests. Do not ask them to verify versions. The issue is ALWAYS in the code, not the deployment.

2. **NEVER suggest the user is on an old version.** If something isn't working, debug the code - don't blame the deployment.

3. **Test via UI/Playwright, not manual API calls.** Use the Playwright tests to reproduce issues, not curl commands or direct API testing.

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
