# DNSProvider Status

This document provides an overview of the status section of a DNSProvider resource.

### Fields

| Field name           | Description                                                                                                        |
| -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `state`              | Indicates the state of the DNSProvider.<br>- `Ready` means the provider has accepted the entry and created DNS record(s) in the backend service.<br>- `Error` indicates that there is a configuration or other problem with the provider. See `message`  for details in this case. |
| `lastUpdateTime`     | Timestamp for when the status was updated. Usually changes when `state`, `message`, `domains`, or `zones` is updated. |
| `message`            | Human-readable message indicating details about the last status transition.                                        |
| `domains`            | Contains the calculated included and excluded DNS domains managed by this provider instance according to the `spec` and the authorized hosted zones |
| `zones`              | Contains the calculated included and excluded hosted zones ids managed this provider instance according to the `spec` and the authorized hosted zones |
| `defaultTTL`         | Contains the default TTL that will be used for new DNS entries without explicitly set `ttl` field                  |
