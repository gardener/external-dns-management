# DNSEntry Status

This document provides an overview of the status section of a DNSEntry resource.

### Fields

| Field name             | Description                                                                                                                                             |
|------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| `state`                | Indicates the state of the DNSEntry. Details see below.                                                                                                 |
| `cnameLookupInterval`  | Shows effective lookup interval for targets domain names to be resolved to IP addresses. Only provided if lookups are active for this entry.            |
| `lastUpdateTime`       | Timestamp for when the status was updated. Usually changes when any relevant status field like `state`, `message`, `provider`, or `targets` is updated. |
| `message`              | Human-readable message indicating details about the last status transition.                                                                             |
| `provider`             | Shows the DNS provider assigned to this entry.                                                                                                          |
| `providerType`         | Shows the DNS provider type assigned to this entry.                                                                                                     |
| `targets`              | Shows the stored targets or text of the DNS record in the backend service.                                                                              |
| `ttl`                  | Shows the stored TTL value of the DNS record in the backend service.                                                                                    |

Currently the available states are:

- `Ready` means the provider has accepted the entry and created DNS record(s) in the backend service.
- `Pending` means the update of the DNS records in the DNS backend service is batched or in progress.
- `Error` means there is configuration or other problem. See `message` for details in this case.
- `Invalid` means there is a conflict with another DNS entry or owner. See `message` for details in this case.
- `Stale` means the DNS records in the backend service are existing but there is a problem with the provider. See `message` for details in this case.
- `Deleting` means the deletion of the DNS records in the DNS backend service is in progress.
- `Ignored` means the entry is annotated with `dns.gardener.cloud/ignore=true|reconcile|full`. For values `true` and `reconcile`, the reconciliation is skipped. `true` is an alias for `reconcile`. For value `full` both reconciliation and deletion operations are skipped. 
- An empty state ` ` means that no matching provider has been found.