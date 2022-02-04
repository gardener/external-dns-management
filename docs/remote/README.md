# Remote DNS Provider

This DNS provider allows you to create and manage DNS entries via a remote `dns-controller-manager` instance.

## Client-side

A `DNSProvider` on the client side uses the type `remote` and a secret with the configuration to access the remote instance.

### Credentials

The communication between the local (client) and the remote (server) `dns-controller-manager` uses mTLS.
Therefore both sides must be configured using TLS certificates of a known CA.

These are the supported keys:

- `REMOTE_ENDPOINT` - "<host>:<port>" of the remote-access service running on the remote dns-controller-manager.
- `NAMESPACE` - <namespace> of the remote cluster. All included zones of all namespace's DNSProvider objects annotated with 'dns.gardener.cloud/remote-access=true' are available. 
- `OVERRIDE_SERVER_NAME` - optionally overrides server name as specified in the server certificate (if server cannot be accessed with the DNS name/IP address as specified in the TLS certificate)
- `ca.crt` or `SERVER_CA_CERT` - CA used for the server certificate
- `tls.crt` or `CLIENT_CERT` - client certificate
- `tls.key` or `CLIENT_KEY` - private key of the client certificate

### Using the Credentials

Create a `Secret` resource with the complete set of keys .
All values are base64 encoded.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: remote-credentials
  namespace: default
type: Opaque
data:
  # Replace '...' with values encoded as base64.
  REMOTE_ENDPOINT: ...  # "<host>:<port>" of the remote-access service running on the remote dns-controller-manager
  NAMESPACE: ... # <namespace> of the remote cluster. All included zones of all namespace's DNSProvider objects annotated with 'dns.gardener.cloud/remoteAccess=true' are available.
  #OVERRIDE_SERVER_NAME: ... # optional override server name as specified in the server certificate
  ca.crt: ... # CA used for the server certificate
  tls.crt: ... # client certificate
  tls.key: ... # client private key
``` 

## Server-side

The remote `dns-controller-manager` instance must run with enabled remote access (see `--remote-access-*` command line 
options for more details) and must expose an endpoint with the remote access service.
If you use the Helm chart, see the `remoteaccess` section in the values file (e.g. ../charts/external-dns-management/values.yaml):

```yaml
remoteaccess:
  service:
    annotations:
      #dns.gardener.cloud/class: garden
      dns.gardener.cloud/dnsnames: my.foo.bar.com
    type: LoadBalancer
  enabled: true
  certs:
    cacert: LS0t...
#    cakey: LS0t... # only needed if remoteaccesscertificates controller is enabled
    servercert: LS0t...
    serverkey: LS0t...
  port: 7777
```

`DNSProvider` objects are defined normally with any provider type. Only providers annotated with `dns.gardener.cloud/remote-access=true` can be accessed
remotely using a `DNSProvider` of type `remote`.
Additionally, depending on the common name of the client certificate, only providers of one namespace may be accessible.

1. Example:
A common name `default.my.client` restricts the client to providers in namespace `default`.

2. Example:
A common name `*.my.second.client` allows access to all providers in all namespaces.


