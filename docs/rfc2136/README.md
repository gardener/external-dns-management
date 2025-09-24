# RFC2136 DNS Provider (alpha)

This DNS provider allows you to create and manage DNS entries for authoritive DNS server supporting
dynamic updates with DNS messages following [RFC2136](https://datatracker.ietf.org/doc/html/rfc2136) (DNS Update)
like [knot-dns](https://www.knot-dns.cz/) or others.

*Note: RFC2136 has no support for hosted zone management, only one single zone can be specified per `DNSProvider`.*

*Note: This provider is in alpha state and it is not recommended to be used in production.*

## DNS Server configuration

The configuration is depending on the DNS server product.
You need permissions for `update` and `transfer` (AXFR) actions on your zones and a TSIG secret.

Here it is described for the "Knot DNS Server" only.

### Configuration of Knot DNS Server

The `knot.conf` must contain a key which has ACLs for `update` and `transfer` actions.


```yaml
# knot.conf sample configuration excerpt

key:
  - id: "my-key."
    algorithm: hmac-sha256
    secret: ...

acl:
  - id: "update-acl"
    address: [ "192.168.0.0/16" ] # replace with your address filter
    action: update
    key: "my-key."
  - id: "transfer-acl"
    address: [ "192.168.0.0/16" ] # replace with your address filter
    action: transfer
    key: "my-key."

zone:
  - domain: example.com
    acl: [ "update-acl", "transfer-acl" ]
```

see also [KNOT DNS Configuration](https://www.knot-dns.cz/docs/3.3/html/configuration.html)

## Required permissions

There are no special permissions for the access tokens.

## Using the TSIG secret and server settings 

Create a `Secret` resource with the following data fields.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: rfc2136-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with values encoded as base64
  Server: ... # "<host>[:<port>]" of the authoritative DNS server, default port is 53
  TSIGKeyName: ... # key name of the TSIG secret (must end with a dot)
  TSIGSecret: ... # TSIG secret
  Zone: ... # zone (must be fully qualified)
  # the algorithm is optional. By default 'hmac-sha256' is assumed.
  #TSIGSecretAlgorithm: ... # TSIG Algorithm Name for Hash-based Message Authentication (HMAC)
``` 

The field `TSIGSecretAlgorithm` is optional and specifies the TSIG Algorithm Name for Hash-based Message Authentication (HMAC).