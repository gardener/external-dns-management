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
    Requeue["Requeue"]
    LockName("Lock DNSName")
    CalcNewProvider("Calculate New Provider")
    UpdateStatus("UpdateStatus")
    StateReady>"State=Ready"]
    StateStale>"State=Stale"]
    StateError>"State=Error"]
    Validate("Validate DNSEntry spec")
    CalcOldTargets("Calculate old targets
    from DNSEntry status")
    CalcNewTargets("Calculate new targets
    from DNSEntry spec")
    QueryRecords("Query records
    from authoritative DNS server
    or via API")
    CalculateChanges("Calculate change requests
    (per zone)")
    ApplyChangeRequests("Apply change requests
    (if any)")
    NoMatchingTxt[/"no matching DNS provider found"/]
    ProviderNotReadyTxt[/"provider has status ≪provider status message≫
    or is not ready yet"/]
    
    Start --> Get
    Get -->|ok| LockName
    Get -->|ignored by annotation| Stop
    LockName -->|ok| Validate
    LockName -->|"already locked by
    another reconciliation"| Requeue --> Stop
    Validate --> CalcNewProvider
    CalcNewProvider -->|ok| CalcOldTargets
    CalcNewProvider -->|not found| NoMatchingTxt
    NoMatchingTxt -->|"No targets in .status"| StateError
    NoMatchingTxt -->|"Existing targets in .status"| StateStale
    CalcNewProvider -->|not ready| ProviderNotReadyTxt --> StateStale
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
