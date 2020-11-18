# Gardener DNS Management for Shoots

## Introduction
Gardener allows Shoot clusters to request DNS names for Ingresses and Services out of the box. 
To support this the gardener must be installed with the `shoot-dns-service`
extension.
This extension uses the seed's dns management infrastructure to maintain DNS
names for shoot clusters. So, far only the external DNS domain of a shoot
(already used for the kubernetes api server and ingress DNS names) can be used
for managed DNS names.

<style>
#body-inner blockquote {
    border: 0;
    padding: 10px;
    margin-top: 40px;
    margin-bottom: 40px;
    border-radius: 4px;
    background-color: rgba(0,0,0,0.05);
    box-shadow: 0 3px 6px rgba(0,0,0,0.16), 0 3px 6px rgba(0,0,0,0.23);
    position:relative;
    padding-left:60px;
}
#body-inner blockquote:before {
    content: "!";
    font-weight: bold;
    position: absolute;
    top: 0;
    bottom: 0;
    left: 0;
    background-color: #00a273;
    color: white;
    vertical-align: middle;
    margin: auto;
    width: 36px;
    font-size: 30px;
    text-align: center;
}
</style>

## Configuration

A general description for configuring the DNS management of the
gardener can be found [here](https://github.com/gardener/gardener/blob/master/docs/extensions/dns.md).

To generally enable the DNS management for shoot objects the 
`shoot-dns-service` extension must be registered by providing an
appropriate [extension registration](https://github.com/gardener/gardener-extension-shoot-dns-service/blob/master/example/controller-registration.yaml) in the garden cluster.

Here it is possible to decide whether the extension should be always available
for all shoots or whether the extension must be separately enabled per shoot.

If the extension should be used for all shoots the registration must set the *globallyEnabled* flag to `true`.

```yaml
spec:
  resources:
    - kind: Extension
      type: shoot-dns-service
      globallyEnabled: true
```

### Providing Base Domains usable for a Shoot

So, far only the external DNS domain of a shoot already used
for the kubernetes api server and ingress DNS names can be used for managed
DNS names. This is either the shoot domain as subdomain of the default domain
configured for the gardener installation or a dedicated domain with dedicated
access credentials configured for a dedicated shoot via the shoot manifest.

### Shoot Feature Gate

If the shoot DNS feature is not globally enabled by default (depends on the 
extension registration on the garden cluster), it must be enabled per shoot.

To enable the feature for a shoot, the shoot manifest must explicitly add the
`shoot-dns-service` extension.

```yaml
...
spec:
  extensions:
    - type: shoot-dns-service
...
```

