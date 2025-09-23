# Infoblox DNS Provider

> [!WARNING]
> The Infoblox DNS provider is deprecated and will be removed in future releases end of 2025, as there is 
> no maintainer and not test environment available anymore. Please consider using an alternative DNS provider.

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
  name: infoblox-credentials
  namespace: default
type: Opaque
data:
  USERNAME: bXktdGVjaG5pY2FsLXVzZXI=
  PASSWORD: bXktcGFzc3dvcmQ=
  # The providerConfig parameters of the DNS provider can be specified here alternatively (not recommenended)
  #HOST: MTAuMTEuMjMuNDU=
  #PORT: NDQz
  #VERSION: Mi4xMA==
  #VIEW: ZGVmYXVsdA==
  #SSL_VERIFY: ZmFsc2U=
  #HTTP_POOL_CONNECTIONS: MTA=
  #HTTP_REQUEST_TIMEOUT: NjA=
  #PROXY_URL: aHR0cDovLzEwLjEuMi4zOjg4ODg=
```

## Create DNS provider

The Infoblox `DNSProvider` needs additional parameters as `providerConfig`. The `host` parameter is required.
The `version` parameter is defaulted to `2.10`, but can be overwritten. All others are optional.

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
    
    # sslVerify is the flag to use HTTPS for API request. Set to true by default.
    #sslVerify: true

    # version is the api version. Set to "2.10" by default.
    #version: "2.10"
   
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

    # extensible attributes to add to each DNS record (used when "Cloud Network Automation" is enabled)
    extAttrs:
      "key1": "value1"
      "key2": "value2"
  domains:
    include:
    - my.own.domain.com
```
