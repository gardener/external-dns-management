# Remote DNS Provider

This DNS provider allows you to create and manage DNS entries via a remote `dns-controller-manager` instance.
The remote `dns-controller-manager` instance must run with enabled remote access (all `--remote-access-*` command line options).
This remote instance is configured normally using `DNSProvider` objects. Any of these objects annotated with
`dns.gardener.cloud/remoteAccess=true` can be accessed locally using a `DNSProvider` of type `remote`.

## Credentials

The communication between the local (client) and the remote (server) `dns-controller-manager` uses mTLS.
Therefore both sides must be configured using TLS certificates of a known CA.

These are the supported keys:

- `REMOTE_ENDPOINT` - "<host>:<port>" of the remote-access service running on the remote dns-controller-manager.
- `NAMESPACE` - <namespace> of the remote cluster. All included zones of all namespace's DNSProvider objects annotated with 'dns.gardener.cloud/remoteAccess=true' are available. 
- `OVERRIDE_SERVER_NAME` - optionally overrides server name as specified in the server certificate (if server cannot be accessed with the DNS name/IP address as specified in the TLS certificate)
- `SERVER_CA_CERT` - CA used for the server certificate
- `CLIENT_CERT` - client certificate
- `CLIENT_KEY` - private key of the client certificate

## Using the Credentials

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
  SERVER_CA_CERT: ... # CA used for the server certificate
  CLIENT_CERT: ... # client certificate
  CLIENT_KEY: ... # client private key
``` 
