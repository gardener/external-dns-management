# Controller-Runtime-Rewrite

This folder contains documentation for the next major version of the dns-controller-manager,
which is being rewritten using the [controller-runtime](sigs.k8s.io/controller-runtime).
The rewrite is still in progress and not yet released.

Additionally, it will contain some major changes to the architecture and features of the dns-controller-manager.

## Major Changes
- On reconciling `DNSEntry`, the current state is retrieved by DNS queries to the authoritative nameservers, which are used instead of the API endpoints of the DNS providers. This saves lots of calls to the API endpoints and makes existing rate limits much less problematic.
- No support for `InfoBlox` and `Remote` DNS providers anymore
- No caching of zone state needed anymore.

## Development

See here for the current reconciliation logic for `DNSEntries` and `DNSProviders`:
- [Reconciliation of `DNSEntries`](development/dnsentry-reconciliation.md)
- [Reconciliation of `DNSProviders`](development/dnsprovider-reconciliation.md)