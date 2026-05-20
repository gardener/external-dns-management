# Metrics (Next-Generation dns-controller-manager)

The next-generation dns-controller-manager exposes [Prometheus](https://prometheus.io/) metrics on a dedicated HTTP endpoint. By default the metrics server listens on port **2753** (configurable via `server.metrics.port` in the `DNSManagerConfiguration`).

**Endpoint:** `GET /metrics`

All metric names are prefixed with `external_dns_management_`.

## Provider & Account Metrics

| Name | Type | Labels | Description |
|------|------|--------|-------------|
| `external_dns_management_account_providers` | Gauge | `providertype`, `accounthash` | Current number of `DNSProvider` resources active for a given provider type and credential set. Updated whenever a provider is added, removed, or its credentials change. Removed when the account is deleted. |
| `external_dns_management_total_provider_requests` | Counter | `providertype`, `accounthash`, `requesttype` | Cumulative number of API calls made to a DNS provider, broken down by provider type, credential set, and request type (e.g. `list_zones`, `list_records`, `update_records`, `create_records`, `delete_records`, `cached_getzones`). |
| `external_dns_management_requests_per_zone` | Counter | `providertype`, `accounthash`, `requesttype`, `zone` | Same as `external_dns_management_total_provider_requests` but further broken down by the DNS zone that was targeted. Only incremented when a zone context is available. |

## Zone Metrics

The next-generation dns-controller-manager periodically lists all `DNSEntry` resources and aggregates them by hosted zone and reconciliation state. The interval is controlled by `controllers.dnsEntry.zoneMetricsInterval` in the `DNSManagerConfiguration` (default: 30 seconds; set to `0` to disable).

| Name | Type | Labels | Description |
|------|------|--------|-------------|
| `external_dns_management_dns_entries` | Gauge | `providertype`, `zone`, `state` | Current number of DNS entries (`DNSEntry` resources) grouped by their target hosted zone and the value of `status.state` (e.g. `Ready`, `Pending`, `Error`, `Invalid`, `Stale`, or empty if not yet set). Entries that are not yet bound to a zone (`status.zone` not set) are reported with `providertype=""` and `zone=""`. Stale label combinations are removed automatically when no entry matches them anymore. |

## Lookup Processor Metrics

The lookup processor resolves hostnames referenced in `DNSEntry` resources (targets using DNS names instead of IP addresses) and queues re-reconciliations when results change.

| Name | Type | Labels | Description |
|------|------|--------|-------------|
| `external_dns_management_lookup_processor_jobs` | Gauge | _(none)_ | Current number of pending jobs in the lookup processor queue. |
| `external_dns_management_lookup_processor_skips` | Counter | _(none)_ | Cumulative number of hostname lookups skipped because the processor was overloaded. An increasing value indicates backpressure on the lookup queue. |
| `external_dns_management_lookup_processor_lookups` | Counter | `namespace` | Cumulative number of hostname lookup cycles executed per namespace. Each cycle resolves all hostnames for one `DNSEntry`. |
| `external_dns_management_lookup_processor_hosts` | Counter | `namespace` | Cumulative number of individual hostname resolution calls made per namespace. A single lookup cycle may resolve multiple hostnames. |
| `external_dns_management_lookup_processor_errors` | Counter | `namespace` | Cumulative number of failed hostname resolution calls per namespace. Compare with `external_dns_management_lookup_processor_hosts` to derive an error rate. |
| `external_dns_management_lookup_processor_lookup_changed` | Counter | `namespace` | Cumulative number of lookup cycles where the resolved addresses differed from the previously cached result. Each increment triggers a re-reconciliation of the affected `DNSEntry`. |
| `external_dns_management_lookup_processor_seconds` | Histogram | _(none)_ | Duration of individual hostname lookup calls in seconds. Buckets: 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20. Use this to detect slow upstream DNS resolvers. |

## Label Reference

| Label | Description |
|-------|-------------|
| `providertype` | DNS provider type, e.g. `aws-route53`, `google-clouddns`, `azure-dns` |
| `accounthash` | Hash of the provider credentials, used to distinguish accounts of the same provider type without exposing secrets |
| `requesttype` | API operation performed against the provider, e.g. `list_zones`, `list_records`, `update_records`, `create_records`, `delete_records`, `cached_getzones` |
| `zone` | Provider-specific zone identifier (empty for entries not yet bound to a zone) |
| `namespace` | Kubernetes namespace of the `DNSEntry` resource |
| `state` | Value of `status.state` of the `DNSEntry` (e.g. `Ready`, `Pending`, `Error`, `Invalid`, `Stale`, `Ignored`; empty string if not set) |

## Metrics Not Present in the Next-Generation Version

The following metrics exist in the **legacy** dns-controller-manager but are **not registered** in the next-generation version:

| Name | Description                                    |
|------|------------------------------------------------|
| `external_dns_management_remoteaccess_logins` | Remote access feature not present in next-gen  |
| `external_dns_management_remoteaccess_requests` | Remote access feature not present in next-gen  |
| `external_dns_management_remoteaccess_seconds` | Remote access feature not present in next-gen  |
| `external_dns_management_remoteaccess_transport_credentials` | Remote access feature not present in next-gen  |
| `external_dns_management_entry_reconciliations_total` | Not implemented in next-gen              |
| `external_dns_management_zone_reconciliations_total` | There are no zone reconciliations in next-gen |
| `external_dns_management_zone_reconciliations_seconds` | There are no zone reconciliations in next-gen |
| `external_dns_management_zone_cache_discardings` | The next-gen does not maintain a per-zone in-memory cache that gets discarded |
| `external_dns_management_dns_entries_stale` | Replaced by the `state` label on `external_dns_management_dns_entries` |