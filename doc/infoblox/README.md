# Infoblox DNS Provider

This DNS provider allows you to create and manage DNS entries in [Infoblox DNS](https://www.infoblox.com/products/dns/). 

Please note, that this controller must be activated explicitly.
This means on starting the dns-controller-manager you need to provide the argument `--controllers=infoblox-dns` or
`--controllers=dnscontrollers,infoblox-dns` if you want to activate all providers.

## Create secret with Infoblox credentials

Create a `Secret` resource with `data.USER` and `data.PASSWORD` to be the base64 encoded (technical) user and password.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudflare-credentials
  namespace: default
type: Opaque
data:
  USERNAME: bXktdGVjaG5pY2FsLXVzZXI=
  PASSWORD: bXktcGFzc3dvcmQ=
```

## Create DNS provider

The Infoblox `DNSProvider` needs additional parameters as `providerConfig`. `host` and `version` parameters is required.
All others are optional.

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: infoblox
  namespace: default
spec:
  type: infoblox-dns
  secretRef:
    name: infoblox-credentials
  providerConfig:
    host: 10.11.23.45
    #port: 443
    
    # sslVerify is the flag to use HTTPS for API reqeust. Set to true by default.
    #sslVerify: true

    # version is the api version
    version: "2.10"
   
    # view is the Infoblox DNS view to use
    #view: default

    # httpPoolConnections is the size of the connection pool
    #httpPoolConnections: 10

    # request timeout in seconds
    #httpRequestTimeout: 60

    # cacert is an inlined certificate. Only needed if sslVerify = true and use of self-signed/internal certificate 
    #caCert:

    # max results = 0 means unrestricted
    #maxResults: 0
   
    # proxyUrl is only needed if Infoblox is reachable only via proxy
    #proxyUrl: http://10.1.2.3:8888
  domains:
    include:
    - my.own.domain.com
```
