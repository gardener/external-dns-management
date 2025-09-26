# `DNSEntry` reconciliation

The `reconcile` method in the code handles DNS entry reconciliation by:
1. Managing DNS provider selection and validation
2. Locking DNS names to prevent concurrent modifications
3. Calculating and comparing old vs new DNS targets
4. Applying necessary changes to DNS records
5. Updating the DNSEntry status based on the reconciliation results

It's part of a DNS controller that ensures DNS entries are properly managed through different DNS providers while handling state transitions, routing policies, and record updates.

```mermaid
---
title: DNSEntry reconciliation
config:
  flowchart:
    htmlLabels: false
---
flowchart TD
    Get("`Get **DNSEntry**`")
    CheckIgnored{"Has
    ignore
    annotation?"}
    Requeue["Requeue"]
    LockName{"DNSName
    locked
    successfully?"}
    CalcNewProvider("Calculate New Provider")
    ProviderFound{"Provider found?"}
    ProviderReady{"Provider ready?"}
    UpdateStatus("UpdateStatus")
    StateReady>"State=Ready"]
    StateStale>"State=Stale"]
    TargetsInStatus{"Has targets in\nDNSEntry status?"}
    StateError>"State=Error"]
    Validate("Validate DNSEntry spec")
    CalcOldTargets("Calculate old targets
    from DNSEntry status")
    CalcNewTargets("Calculate new targets
    from DNSEntry spec")
    QueryRecords("Query records\nfrom authoritative DNS server
    or via API")
    CalculateChanges("Calculate change requests
    (per zone)")
    ApplyChangeRequests("Apply change requests
    (if any)")
    NoMatchingTxt1[/"no matching DNS provider found"/]
    NoMatchingTxt2[/"no matching DNS provider found"/]
    ProviderNotReadyTxt[/"provider has status ...
    or is not ready yet"/]
    
    Start --> Get --> CheckIgnored
    CheckIgnored -->|no| LockName
    CheckIgnored -->|yes| Stop
    LockName -->|yes| CalcNewProvider
    LockName -->|already locked by\nanother reconciliation| Requeue --> Stop
    CalcNewProvider --> ProviderFound
    ProviderFound -->|yes| ProviderReady
    ProviderFound -->|no| TargetsInStatus
    TargetsInStatus -->|yes| NoMatchingTxt1 --> StateStale
    TargetsInStatus -->|no| NoMatchingTxt2 --> StateError
    ProviderReady -->|yes| Validate --> CalcOldTargets
    ProviderReady -->|no| ProviderNotReadyTxt --> StateStale
    CalcOldTargets --> CalcNewTargets
    CalcNewTargets --> QueryRecords
    QueryRecords --> CalculateChanges
    CalculateChanges --> ApplyChangeRequests
    ApplyChangeRequests --> StateReady
    StateStale --> UpdateStatus
    StateError --> UpdateStatus
    StateReady --> UpdateStatus
    UpdateStatus --> Stop
```
