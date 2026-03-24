# Controller-Runtime-Rewrite

This folder contains documentation for the next generation of the dns-controller-manager,
which is being rewritten using the [controller-runtime](https://sigs.k8s.io/controller-runtime).
The rewrite is mostly completed. Support for annotations on Istio Gateways is not yet merged, and the rewrite is not yet recommended for production usage.

Additionally, it contains some major changes to the architecture and features of the dns-controller-manager.

## Major Changes
- On reconciling `DNSEntry`, the current state is retrieved by DNS queries to the authoritative nameservers, which are used instead of the API endpoints of the DNS providers. This significantly reduces calls to the API endpoints and makes existing rate limits much less problematic.
  As a consequence, the dns-controller-manager deployment needs to have network access to the authoritative nameservers of the zones managed by the DNS providers additionally to the API endpoints of the DNS providers.
- There is no separate zone reconciliation loop anymore, the `DNSEntries` are directly applied during their reconciliation. This simplifies the logic and makes troubleshooting easier, as everything happens during the `DNSEntry` reconciliation, and there is no need to correlate logs of different controllers anymore.
- The configuration of the `dns-controller-manager` uses a configuration file instead of command line arguments. 
- No support for `Infoblox` and `Remote` DNS providers anymore.
- No caching of zone state needed anymore.
- The `DNSEntry` field `.spec.reference` is not supported anymore.

| Aspect                      | Legacy                          | Next-Generation                                    |
|-----------------------------|---------------------------------|----------------------------------------------------|
| Network requirements        | Provider APIs only              | Provider APIs + authoritative nameservers          |
| API calls                   | High (queries provider APIs)    | Low (queries DNS directly)                         |
| Reconciliation loops        | Separate for zones and entries  | Single loop for applying DNS records for entries   |
| Configuration               | Command line arguments          | Configuration file                                 |
| `infoblox` provider type    | ✅ Supported                    | ❌ Not supported                                    |
| `remote` provider type      | ✅ Supported                    | ❌ Not supported                                    |
| Zone state caching          | ✅ Uses cached state            | ❌ Queries authoritative nameservers                |
| `.spec.reference`           | ✅ Supported                    | ❌ Not supported                                    |

## Configuration

The next-gen version of the dns-controller-manager uses a configuration file instead of command line arguments for most settings.

### Usage
To run the next-gen dns-controller-manager, you can provide these flags on the command line:

```bash
Usage:
  dns-controller-manager-next-generation [flags]

Flags:
      --config string                 Path to configuration file.
      --controllers strings           List of controllers to enable. If not set, all controllers are enabled. [gatewayapiv1beta1-source gatewayapiv1-source crdwatch-source dnsprovider dnsentry dnsannotation ingress-source dnsprovider-source service-source dnsentry-source] (default [gatewayapiv1beta1-source,gatewayapiv1-source,crdwatch-source,dnsprovider,dnsentry,dnsannotation,ingress-source,dnsprovider-source,service-source,dnsentry-source])
      --disable-controllers strings   List of controllers to disable [gatewayapiv1beta1-source gatewayapiv1-source crdwatch-source dnsprovider dnsentry dnsannotation ingress-source dnsprovider-source service-source dnsentry-source]
  -h, --help                          help for dns-controller-manager-next-generation
      --v                             If true, overwrites log level in config with value 'debug'.
      --version version[=true]        --version, --version=raw prints version information and quits; --version=vX.Y.Z... sets the reported version
```
### Default Configuration

If an empty configuration file is provided like this:

```yaml
apiVersion: config.dns.gardener.cloud/v1alpha1
kind: DNSManagerConfiguration
```

The following default configuration will be used.
*Please check the printed configuration in the startup logs for the most recent defaults, as they might change during development*:

```yaml
class: gardendns
clientConnection:
  acceptContentTypes: ""
  burst: 130
  cacheResyncPeriod: 1h0m0s
  contentType: ""
  kubeconfig: ""
  qps: 100
controlPlaneClientConnection:
  acceptContentTypes: ""
  burst: 130
  cacheResyncPeriod: 1h0m0s
  contentType: ""
  kubeconfig: ""
  qps: 100
controllers:
  dnsAnnotation: {}
  dnsEntry:
    concurrentSyncs: 5
    reconciliationTimeout: 2m0s
    syncPeriod: 1h0m0s
  dnsProvider:
    concurrentSyncs: 2
    defaultRateLimits:
      burst: 20
      enabled: true
      qps: 10
    defaultTTL: 300
    gcpWorkloadIdentityConfig:
      allowedServiceAccountImpersonationURLRegExps:
      - ^https://iamcredentials\\.googleapis\\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$
      allowedTokenURLs:
      - https://sts.googleapis.com/v1/token
    namespace: default
    recheckPeriod: 5m0s
    reconciliationTimeout: 2m0s
    syncPeriod: 1h0m0s
    zoneCacheTTL: 30m0s
  source:
    concurrentSyncs: 5
    targetNamespace: default
leaderElection:
  leaderElect: true
  leaseDuration: 15s
  renewDeadline: 10s
  resourceLock: leases
  resourceName: dns-controller-manager-controllers
  resourceNamespace: default
  retryPeriod: 2s
logFormat: json
logLevel: info
server:
  healthProbes:
    bindAddress: ""
    port: 2751
  metrics:
    bindAddress: ""
    port: 2753
```

### Example Configuration File
Here is an example configuration file for the next-gen dns-controller-manager, using separate source and control plane kubeconfigs, custom log format and level, and a custom namespace for the `DNSProvider` controller:

```yaml
apiVersion: config.dns.gardener.cloud/v1alpha1
kind: DNSManagerConfiguration
logFormat: text # default is json
logLevel: debug # default is info
clientConnection:
  kubeconfig: /path/to/kubeconfig/of/source/cluster
controlPlaneClientConnection:
  kubeconfig: /path/to/kubeconfig/of/control-plane/cluster
controllers:
  dnsProvider:
    namespace: my-namespace
  source:
    targetNamespace: my-namespace
    targetClusterID: my-target-id
    sourceClusterID: my-source-id
  # dnsProviderReplication: true # if enabled, the dnsProviderReplication controller will be activated, which replicates DNSProviders from the control plane cluster to the source cluster
deployCRDs: true
```

For a complete reference of all configuration options, please check the [API reference](api-reference/dnsmanagerconfiguration.md#config.dns.gardener.cloud/v1alpha1.DNSManagerConfiguration).

## Migration From Legacy Version

The Kubernetes custom resources `DNSProvider`, `DNSEntry`, and `DNSAnnotation` remain unchanged in the next-gen version, but there are some important changes to be aware of when migrating from the legacy version:

1. DNS Class Consistency: Ensure the `dns.gardener.cloud/class` annotation on all resources matches the controller's configured class.
2. Provider Type Support: Check if you're using Infoblox or Remote providers - these are NOT supported in next-gen and require staying on legacy.
3. Network Access: Next-gen requires network access to authoritative nameservers (port 53) in addition to provider APIs.
4. Controller Names Changed: The controller registration uses different names internally but serves the same purpose.
5. `DNSProviders` and `DNSEntries` are only watched in the control plane namespace.

### Controller Name Mapping

In the legacy version, controllers are activated with the `--controllers` flag, which accepts a comma-separated list of controller names. 
In the next-gen version, all controllers are enabled by default. It supports `--controllers` or `--disable-controllers` flags, but with new names. The mapping of legacy controller names to next-gen equivalents is as follows:

| Legacy Controller           | Legacy Group     | Nextgen Equivalent                                      | Nextgen Controller Name                                                 | Config Field(s)                                             |
|-----------------------------|------------------|---------------------------------------------------------|-------------------------------------------------------------------------|-------------------------------------------------------------|
| `compound`                  | `dnscontrollers` | `DNSProvider`, <br/>`DNSEntry` control plane controller | `dnsprovider`,<br/>`dnsentry`                                           | `.controllers.dnsProvider`,<br/>`.controllers.dnsEntry`     |
| `annotation`                |                  | `DNSAnnotation` controller                              | `dnsannotation`                                                         | `.controllers.dnsAnnotation`                                |
| `dnsprovider-replication`   | `replication`    | `DNSProvider` replication                               | `dnsprovider-source`                                                    | `.controllers.source` \[1\]                                 |
| `dnsentry-source`           |                  | `DNSEntry` source controller                            | `dnsentry-source`                                                       | `.controllers.source` \[2\]                                 |
| `ingress-dns`               | `dnssources`     | `Ingress` Source controller                             | `ingress-source`                                                        | `.controllers.source`                                       |
| `service-dns`               | `dnssources`     | `Service` Source controller                             | `service-source`                                                        | `.controllers.source`                                       |
| `istio-gateways-dns`        | `dnssources`     | Source controller (Istio Gateway)                       | `istiov1-source`,<br/>`istiov1beta1-source`,<br/>`istiov1alpha3-source` | `.controllers.source`  *(Istio support PR not yet merged)*  |
| `k8s-gateways-dns`          | `dnssources`     | Source controller (K8s Gateway API)                     | `gatewayapiv1-source`,<br/>`gatewayapiv1beta1-source`                   | `.controllers.source`                                       |
| `watch-gateways-crds`       | `dnssources`     | CRD watcher for K8s/Istio Gateway                       | `crdwatch-source`                                                       | `.controllers.source`                                       |

*Notes:*
- \[1\]: Needs activation with `.controllers.source.dnsProviderReplication: true`
- \[2\]: Automatically disabled if `.controlPlaneClientConnection.kubeconfig` not set and `.class == .controllers.source.sourceClass`

### Mapping of Important Flags

| Legacy Flag                                                                     | Nextgen Equivalent Config Field            | Description                                                                                                                                                                                  |
|---------------------------------------------------------------------------------|--------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--dns-class`                                                                   | `.class`                                   | The DNS class to watch for. Only resources with a matching `dns.gardener.cloud/class` annotation will be processed in the control plane namespace. The default class is `gardendns`.         |
| `--kubeconfig`                                                                  | `.clientConnection.kubeconfig`             | Path to kubeconfig file. If not set, in-cluster configuration will be used.                                                                                                                  |
| `--providers`, `--target`                                                       | `.controlPlaneClientConnection.kubeconfig` | Path to kubeconfig file for control plane (providers and entries). There is no distinction between provider and target cluster anymore.                                                      |
| `--<source>.dns-class`                                                          | `.controllers.source.sourceClass`          | The DNS class to watch for source resources. There is only one source DNS class for all source resources.                                                                                    |
| `--<source>.dns-target-class`                                                   | `.controllers.source.targetClass`          | The DNS class to set for target entries and providers. There is only one target DNS class for all source resources.                                                                          |
| `--<source>.target-namespace`                                                   | `.controllers.source.targetNamespace`      | There is only one target namespace for all source resources.                                                                                                                                 |
| `--<source>.target-name-prefix`                                                 | `.controllers.source.targetNamePrefix`     | There is only one target name prefix for all source resources.                                                                                                                               |
| `--<source>.target-creator-label-name`,<br/>`--<source>.target-creator-label-value` | `.controllers.source.targetLabels`     | Target labels are provided as a map (JSON string) in nextgen.                                                                                                                               |
| `--kubeconfig.id`                                                               | `.controllers.source.sourceClusterID`      | There is only one source cluster ID for all source resources.                                                                                                                                |
| `--target.id`                                                                   | `.controllers.source.targetClusterID`      | There is only one target cluster ID for all source resources.                                                                                                                                |

## Development

See here for the current reconciliation logic for `DNSEntries` and `DNSProviders`:
- [Reconciliation of `DNSEntries`](development/dnsentry-reconciliation.md)
- [Reconciliation of `DNSProviders`](development/dnsprovider-reconciliation.md)