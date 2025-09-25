# PowerDNS Provider

This DNS provider allows you to create and manage DNS entries with [PowerDNS](https://www.powerdns.com/).

## Required permissions

There are no special permissions for the `ApiKey`.

## Credentials

You need to have an `ApiKey` and the url of your PowerDNS `Server` in place.

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
  ApiKey: ... # your PowerDNS API key
  Server: ... # your PowerDNS server url
  #InsecureSkipVerify: ... # true if HTTP is used 
  #TrustedCaCert: ... # CA for HTTPS
  #VirtualHost: ...
```

Supported data keys:

| Key                | Alias              | Description                                                                                    |
|--------------------|--------------------|------------------------------------------------------------------------------------------------|
| ApiKey             | apiKey             | PowerDNS API key                                                                               |
| Server             | server             | PowerDNS server url (must start with https:// or http://)                                      |
| InsecureSkipVerify | insecureSkipVerify | Optional, set to true if your server is accessed via HTTP (not recommended for production use) |
| TrustedCaCert      | trustedCaCert      | Optional CA certificate for PowerDNS server certificate                                        |
| VirtualHost        | virtualHost        | Optional PowerDNS virtual host                                                                 |
```