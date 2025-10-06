# `DNSProvider` reconciliation

The `reconcile` method in this code is responsible for managing DNS provider configurations with the following key steps:

1. Validates the provider type and checks if it's enabled and supported
2. Retrieves and validates secret references and credentials
3. Sets up DNS account configuration with TTL and rate limits
4. Fetches hosted zones from the DNS provider account
5. Updates the provider state and selection based on zones
6. Updates the `DNSProvider` status with current state, zones, TTL, and rate limits

The method ensures proper configuration and state management of DNS providers while handling error cases and validation.

```mermaid
---
title: DNSProvider reconciliation
config:
  flowchart:
    htmlLabels: false
---
flowchart TD
    Get("`Get **DNSProvider**`")
    UpdateStatus("UpdateStatus")
    StateReady>"State=Ready"]
    StateError>"State=Error"]
    StateInvalid>"State=Invalid"]
    GetSecret("`Get provider **Secret**`")
    ValidateCredentialsAndProviderConfig("Validate secret data
    and provider config")
    GetAccount("Get account
    from cache or API")
    GetZones("Get zones")
    Requeue5min["Requeue after 5min"]
    CalcZoneAndDomainSelection("Calculate zone and
    domain selection")
    
    Start --> Get
    Get -->|enabled and supported| GetSecret
    Get -->|not enabled or not supported| StateInvalid
    GetSecret -->|ok| ValidateCredentialsAndProviderConfig
    GetSecret -->|not found| StateError
    ValidateCredentialsAndProviderConfig -->|validation ok| GetAccount
    ValidateCredentialsAndProviderConfig -->|validation failed| StateError
    GetAccount --> GetZones
    GetZones --> CalcZoneAndDomainSelection
    GetZones -->|no zones| Requeue5min --> StateError
    CalcZoneAndDomainSelection --> StateReady
    StateError --> UpdateStatus
    StateReady --> UpdateStatus
    StateInvalid --> UpdateStatus
    UpdateStatus --> Stop
```
