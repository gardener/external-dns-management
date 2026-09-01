# Google Distributed Cloud air-gapped DNS Provider

This DNS provider allows you to create and manage DNS entries in Google Distributed Cloud air-gapped Managed DNS zones.

## Overview

The `gdch-dns` provider synchronizes DNS records with GDC `ManagedDNSZone` and `ResourceRecordSet` custom resources using GDC service account credentials and STS authentication.

### Supported Record Types
The provider supports the following DNS record types: `A`, `CNAME`, `TXT`, `PTR`, `DS`, and `MX`.
> **Note**: `AAAA` (IPv6) records are currently not supported by GDC Managed DNS.

## Required Credentials

You need to provide a Kubernetes secret containing two keys:
1. `serviceaccount.json`: The GDC service account credential JSON.
2. `gdch-config`: Configuration JSON containing the org cluster API URL and CA certificate.

### Example Service Account (`serviceaccount.json`)

```json
{
  "type": "gdch_service_account",
  "name": "dns-sa",
  "project": "my-project",
  "private_key_id": "key-id",
  "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
  "token_uri": "https://identity.org.gdc.example.com/oauth/token"
}
```

### Example GDC Config (`gdch-config`)

```json
{
  "orgClusterURL": "https://global-api.org.gdc.example.com",
  "caData": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg..."
}
```

## Creating Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gdc-credentials
  namespace: default
type: Opaque
data:
  serviceaccount.json: <base64-encoded-serviceaccount.json>
  gdch-config: <base64-encoded-gdch-config>
```

## Creating DNSProvider

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: gdc-provider
  namespace: default
spec:
  type: gdch-dns
  secretRef:
    name: gdc-credentials
  domains:
    include:
      - zone1.google.gdc.example.com
```

## Creating DNSEntry

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: gdc-entry
  namespace: default
spec:
  dnsName: "service.zone1.google.gdc.example.com"
  ttl: 300
  targets:
    - 1.2.3.4
```
