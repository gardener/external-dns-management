# PowerDNS Provider

This DNS provider allows you to create and manage DNS entries with [PowerDNS](https://www.powerdns.com/).

## Required permissions

There are no special permissions for the `apiToken`.

## Credentials

You need to have an `apiToken` and the url of your PowerDNS `server` in place.

Create a `Secret` resource. All credentials need to be base64 encoded.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: powerdns-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values encoded as base64
  apiToken: ... # your PowerDNS token
  server: ... # your PowerDNS server url
```
