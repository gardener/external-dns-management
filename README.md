# External DNS Management

The main artefact of this project is the <b>DNS controller manager</b> for managing DNS records, also
nicknamed as the Gardener "DNS Controller".

It contains provisioning controllers for creating DNS records in one of the DNS cloud services
  - _Amazon Route53_,
  - _Google CloudDNS_,
  - _AliCloud DNS_,
  - _Azure DNS_,
  - _OpenStack Designate_,
  - [_Cloudflare DNS_](/doc/cloudflare/README.md)

and source controllers for services and ingresses to create DNS entries by annotations.

The configuration for the external DNS service is specified in a custom resource `DNSProvider`.
Multiple `DNSProvider` can be used simultaneously and changed without restarting the DNS controller.

DNS records are either created directly for a corresponding custom resource `DNSEntry` or by
annotating a service or ingress.

For a detailed explanation of the model, see section [The Model](#the-model).

For extending or adapting this project with your own source or provisioning controllers, see section
[Extensions](#extensions)

## Quick start

To install the <b>DNS controller manager</b> in your Kubernetes cluster, follow these steps.

1. Prerequisites
    - Check out or download the project to get a copy of the Helm charts.
      It is recommended to check out the tag of the
      [last release](https://github.com/gardener/external-dns-management/releases), so that Helm
      values reference the newest released container image for the deployment.

    - Make sure, that you have installed Helm client (`helm`) locally and Helm server (`tiller`) on
      the Kubernetes cluster. See e.g. [Helm installation](https://helm.sh/docs/install/) for more details.

2. Install the DNS controller manager

    As multiple Gardener DNS controllers can act on the same DNS Hosted Zone concurrently, each instance needs
    an [owner identifier](#owner-identifiers). Therefore choose an identifier sufficiently unique across these instances.

    Then install the DNS controller manager with

    ```bash
    helm install charts/external-dns-management --name dns-controller --namespace=<my-namespace> --set configuration.identifier=<my-identifier>
    ```

    This will use the default configuration with all source and provisioning controllers enabled.
    The complete set of configuration variables can be found in `charts/external-dns-management/values.yaml`.
    Their meaning is explained by their corresponding command line options in section
    [Using the DNS controller manager](#using-the-dns-controller-manager)

    By default, the DNS controller looks for custom resources in all namespaces. The choosen namespace is
    only relevant for the deployment itself.

3. Create a `DNSProvider`

   To specify a DNS provider, you need to create a custom resource `DNSProvider` and a secret containing the
   credentials for your account at the provider. E.g. if you want to use AWS Route53, create a secret and
   provider with

   ```bash
   cat << EOF | kubectl apply -f -
   apiVersion: v1
   kind: Secret
   metadata:
     name: aws-credentials
     namespace: default
   type: Opaque
   data:
     # replace '...' with values encoded as base64
     # see https://docs.aws.amazon.com/general/latest/gr/managing-aws-access-keys.html
     AWS_ACCESS_KEY_ID: ...
     AWS_SECRET_ACCESS_KEY: ...
   EOF
   ```

   and

   ```bash
   cat << EOF | kubectl apply -f -
   apiVersion: dns.gardener.cloud/v1alpha1
   kind: DNSProvider
   metadata:
     name: aws
     namespace: default
   spec:
     type: aws-route53
     secretRef:
       name: aws-credentials
     domains:
       include:
       # this must be replaced with a (sub)domain of the hosted zone
       - my.own.domain.com
   EOF
   ```

   Check the successful creation with

   ```bash
   kubectl get dnspr
   ```

   You should see something like

   ```
   NAME   TYPE          STATUS   AGE
   aws    aws-route53   Ready    12s
   ```

4. Create a `DNSEntry`

   Create an DNS entry with

   ```bash
   cat << EOF | kubectl apply -f -
   apiVersion: dns.gardener.cloud/v1alpha1
   kind: DNSEntry
   metadata:
     name: mydnsentry
     namespace: default
   spec:
     dnsName: "myentry.my-own-domain.com"
     ttl: 600
     targets:
     - 1.2.3.4
   EOF
   ```

   Check the status of the DNS entry with

    ```bash
    kubectl get dnsentry
    ```

    You should see something like

    ```txt
    NAME          DNS                           TYPE          PROVIDER      STATUS    AGE
    mydnsentry    myentry.my-own-domain.com     aws-route53   default/aws   Ready     24s
    ```

    As soon as the status of the entry is `Ready`, the provider has accepted the new DNS record.
    Depending on the provider and your DNS settings and cache, it may take up to a few minutes before
    the domain name can be resolved.

5. Wait for/check DNS record

   To check the DNS resolution, use `nslookup` or `dig`.

   ```bash
   nslookup myentry.my-own-domain.com
   ```

   or with dig

   ```bash
   # or with dig
   dig +short myentry.my-own-domain.com
   ```

   Depending on your network settings, you may get a successful response faster using a public DNS server
   (e.g. 8.8.8.8, 8.8.4.4, or 1.1.1.1)

   ```bash
   dig @8.8.8.8 +short myentry.my-own-domain.com
   ```

For more examples about the custom resources and the annotations for services and ingresses
see the `examples` directory.

## The Model

This project provides a flexible model allowing to
add DNS source objects and DNS provisioning environments by adding
new independent controllers.

There is no single DNS controller anymore. The decoupling between the
handling of DNS source objects, like ingresses or services, and the
provisioning of DNS entries in an external DNS provider like
_Route53_ or _CloudDNS_ is achieved by introducing a new custom resource
`DNSEntry`.

These objects can either be explicitly created to request dedicated DNS
entries, or they are managed based on other resources like ingresses or
services. For the latter dedicated _DNS Source Controllers_ are used.
There might be any number of such source controllers. They do not need to know
anything about the various DNS environments. Their task is to figure out which
DNS entries are required in their realm and manage appropriate `DNSEntry`
objects. From these objects they can also read the provisioning status and
report it back to the original source.

![Model Overview](doc/model.png)

Provisioning of DNS entries in external DNS providers is done by
_DNS Provisioning Controllers_. They don't need to know anything about the
various DNS source objects. They watch `DNSEntry` objects and check whether
they are responsible for such an object. If a provisioning controller feels
responsible for an entry it manages the corresponding settings in the
external DNS environment and reports the provisioning status back to the
corresponding `DNSEntry` object.

To do this a provisioning controller is responsible for a dedicated
environment (for example Route53). For every such environment the controller
uses a dedicated _type_ key. This key is used to look for `DNSProvider` objects.
There might be multiple such objects per environment, specifying the
credentials needed to access different external accounts. These accounts are then
scanned for DNS zones and domain names they support.
This information is then used to dynamically assign `DNSEntry` objects to
dedicated `DNSProvider` objects. If such an assignment can be done by
a provisioning controller then it is _responsible_ for this entry and manages
the corresponding entries in the external environment.
`DNSProvider` objects can specify explicit inclusion and exclusion sets of domain names
and/or DNS zone identifiers to override the scanning results of the account.

### Owner Identifiers

Every DNS Provisioning Controller is responsible for a set of _Owner Identifiers_.
DNS records in an external DNS environment are attached to such an identifier.
This is used to identify the records in the DNS environment managed by a dedicated
controller (manager). Every controller manager hosting DNS Provisioning Controllers
offers an option to specify a default identifier. Additionally there might
be dedicated `DNSOwner` objects that enable or disable additional owner ids.

Every `DNSEntry` object may specify a dedicated owner that is used to tag
the records in the DNS environment. A DNS provisioning controller only acts
of DNS entries it is responsible for. Other resources in the external DNS
environment are not touched at all.

This way it is possbible to
- identify records in the external DNS management environment that are managed
  by the actual controller instance
- distinguish different DNS source environments sharing the same hosted zones
  in the external management environment
- cleanup unused entries, even if the whole resource set is already
  gone
- move the responsibility for dedicated sets of DNS entries among different
  kubernetes clusters or DNS source environments running different
  DNS Provisioning Controller without loosing the entries during the
  migration process.

**If multiple DNS controller instances have access to the same DNS zones, it is very important, that every instance uses a unique owner identifier! Otherwise the cleanup of stale DNS record will delete entries created by another instance if they use the same identifier.**

### DNS Classes

Multiple sets of controllers of the DNS ecosystem can run in parallel in
a kubernetes cluster working on different object set. They are separated by
using different _DNS Classes_. Adding a DNS class annotation to an object of the
DNS ecosytems assigns this object to such a dedicated set of DNS controllers.
This way it is possible to maintain clearly separated set of DNS objects in a
single kubernetes cluster.


## Using the DNS controller manager

The controllers to run can be selected with the `--controllers` option.
Here the following controller groups can be used:
- `dnssources`: all DNS Source Controllers. It includes the conrollers
  - `ingress-dns`: handle DNS annotations for the standard kubernetes ingress resource
  - `service-dns`: handle DNS annotations for the standard kubernetes service resource

- `dnscontrollers`: all DNS Provisioning Controllers. It includes the controllers
  - `alicloud-dns`:
  - `aws-route53`:
  - `azure-dns`:
  - `google-clouddns`:
  - `openstack-designate`:
  - `cloudflare-dns`

- `all`: (default) all controllers

It is also possible to list dedicated controllers by their name.

If a DNS Provisioning Controller is enabled it is important to specify a
unique controller identity using the `--identifier` option.
This identifier is stored in the DNS system to identify the DNS entries
managed by a dedicated controller. There should never be two
DNS controllers with the same identifier running at the same time for the
same DNS domains/accounts.

Here is the complete list of options provided:

```txt
Usage:
  dns-controller-manager [flags]

Flags:
      --alicloud-dns.cache-dir string                               Directory to store zone caches (for reload after restart)
      --alicloud-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache
      --alicloud-dns.default.pool.size int                          Worker pool size for pool default of controller alicloud-dns (default: 2)
      --alicloud-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes
      --alicloud-dns.dns-class string                               Identifier used to differentiate responsible controllers for entries
      --alicloud-dns.dns-delay duration                             delay between two dns reconciliations
      --alicloud-dns.dns.pool.resync-period duration                Period for resynchronization of pool dns of controller alicloud-dns (default: 15m0s)
      --alicloud-dns.dns.pool.size int                              Worker pool size for pool dns of controller alicloud-dns (default: 1)
      --alicloud-dns.dry-run                                        just check, don't modify
      --alicloud-dns.identifier string                              Identifier used to mark DNS entries
      --alicloud-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller alicloud-dns (default: 1)
      --alicloud-dns.providers.pool.resync-period duration          Period for resynchronization of pool providers of controller alicloud-dns (default: 10m0s)
      --alicloud-dns.providers.pool.size int                        Worker pool size for pool providers of controller alicloud-dns (default: 2)
      --alicloud-dns.reschedule-delay duration                      reschedule delay after losing provider
      --alicloud-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller alicloud-dns (default: 2)
      --alicloud-dns.setup int                                      number of processors for controller setup
      --alicloud-dns.ttl int                                        Default time-to-live for DNS entries
      --aws-route53.cache-dir string                                Directory to store zone caches (for reload after restart)
      --aws-route53.cache-ttl int                                   Time-to-live for provider hosted zone cache
      --aws-route53.default.pool.size int                           Worker pool size for pool default of controller aws-route53 (default: 2)
      --aws-route53.disable-zone-state-caching                      disable use of cached dns zone state on changes
      --aws-route53.dns-class string                                Identifier used to differentiate responsible controllers for entries
      --aws-route53.dns-delay duration                              delay between two dns reconciliations
      --aws-route53.dns.pool.resync-period duration                 Period for resynchronization of pool dns of controller aws-route53 (default: 15m0s)
      --aws-route53.dns.pool.size int                               Worker pool size for pool dns of controller aws-route53 (default: 1)
      --aws-route53.dry-run                                         just check, don't modify
      --aws-route53.identifier string                               Identifier used to mark DNS entries
      --aws-route53.ownerids.pool.size int                          Worker pool size for pool ownerids of controller aws-route53 (default: 1)
      --aws-route53.providers.pool.resync-period duration           Period for resynchronization of pool providers of controller aws-route53 (default: 10m0s)
      --aws-route53.providers.pool.size int                         Worker pool size for pool providers of controller aws-route53 (default: 2)
      --aws-route53.reschedule-delay duration                       reschedule delay after losing provider
      --aws-route53.secrets.pool.size int                           Worker pool size for pool secrets of controller aws-route53 (default: 2)
      --aws-route53.setup int                                       number of processors for controller setup
      --aws-route53.ttl int                                         Default time-to-live for DNS entries
      --azure-dns.cache-dir string                                  Directory to store zone caches (for reload after restart)
      --azure-dns.cache-ttl int                                     Time-to-live for provider hosted zone cache
      --azure-dns.default.pool.size int                             Worker pool size for pool default of controller azure-dns (default: 2)
      --azure-dns.disable-zone-state-caching                        disable use of cached dns zone state on changes
      --azure-dns.dns-class string                                  Identifier used to differentiate responsible controllers for entries
      --azure-dns.dns-delay duration                                delay between two dns reconciliations
      --azure-dns.dns.pool.resync-period duration                   Period for resynchronization of pool dns of controller azure-dns (default: 15m0s)
      --azure-dns.dns.pool.size int                                 Worker pool size for pool dns of controller azure-dns (default: 1)
      --azure-dns.dry-run                                           just check, don't modify
      --azure-dns.identifier string                                 Identifier used to mark DNS entries
      --azure-dns.ownerids.pool.size int                            Worker pool size for pool ownerids of controller azure-dns (default: 1)
      --azure-dns.providers.pool.resync-period duration             Period for resynchronization of pool providers of controller azure-dns (default: 10m0s)
      --azure-dns.providers.pool.size int                           Worker pool size for pool providers of controller azure-dns (default: 2)
      --azure-dns.reschedule-delay duration                         reschedule delay after losing provider
      --azure-dns.secrets.pool.size int                             Worker pool size for pool secrets of controller azure-dns (default: 2)
      --azure-dns.setup int                                         number of processors for controller setup
      --azure-dns.ttl int                                           Default time-to-live for DNS entries
      --cloudflare-dns.cache-dir string                             Directory to store zone caches (for reload after restart)
      --cloudflare-dns.cache-ttl int                                Time-to-live for provider hosted zone cache
      --cloudflare-dns.default.pool.size int                        Worker pool size for pool default of controller cloudflare-dns (default: 2)
      --cloudflare-dns.disable-zone-state-caching                   disable use of cached dns zone state on changes
      --cloudflare-dns.dns-class string                             Identifier used to differentiate responsible controllers for entries
      --cloudflare-dns.dns-delay duration                           delay between two dns reconciliations
      --cloudflare-dns.dns.pool.resync-period duration              Period for resynchronization of pool dns of controller cloudflare-dns (default: 15m0s)
      --cloudflare-dns.dns.pool.size int                            Worker pool size for pool dns of controller cloudflare-dns (default: 1)
      --cloudflare-dns.dry-run                                      just check, don't modify
      --cloudflare-dns.identifier string                            Identifier used to mark DNS entries
      --cloudflare-dns.ownerids.pool.size int                       Worker pool size for pool ownerids of controller cloudflare-dns (default: 1)
      --cloudflare-dns.providers.pool.resync-period duration        Period for resynchronization of pool providers of controller cloudflare-dns (default: 10m0s)
      --cloudflare-dns.providers.pool.size int                      Worker pool size for pool providers of controller cloudflare-dns (default: 2)
      --cloudflare-dns.reschedule-delay duration                    reschedule delay after losing provider
      --cloudflare-dns.secrets.pool.size int                        Worker pool size for pool secrets of controller cloudflare-dns (default: 2)
      --cloudflare-dns.setup int                                    number of processors for controller setup
      --cloudflare-dns.ttl int                                      Default time-to-live for DNS entries
      --cache-dir string                                            default for all controller "cache-dir" options
      --cache-ttl int                                               default for all controller "cache-ttl" options
  -c, --controllers string                                          comma separated list of controllers to start (<name>,source,target,all) (default "all")
      --cpuprofile string                                           set file for cpu profiling
      --disable-namespace-restriction                               disable access restriction for namespace local access only
      --disable-zone-state-caching                                  default for all controller "disable-zone-state-caching" options
      --dns-class string                                            default for all controller "dns-class" options
      --dns-delay duration                                          default for all controller "dns-delay" options
      --dns-target-class string                                     default for all controller "dns-target-class" options
      --dnsentry-source.default.pool.resync-period duration         Period for resynchronization of pool default of controller dnsentry-source (default: 2m0s)
      --dnsentry-source.default.pool.size int                       Worker pool size for pool default of controller dnsentry-source (default: 2)
      --dnsentry-source.dns-class string                            identifier used to differentiate responsible controllers for entries
      --dnsentry-source.dns-target-class string                     identifier used to differentiate responsible dns controllers for target entries
      --dnsentry-source.exclude-domains stringArray                 excluded domains
      --dnsentry-source.key string                                  selecting key for annotation
      --dnsentry-source.target-creator-label-name string            label name to store the creator for generated DNS entries
      --dnsentry-source.target-creator-label-value string           label value for creator label
      --dnsentry-source.target-name-prefix string                   name prefix in target namespace for cross cluster generation
      --dnsentry-source.target-namespace string                     target namespace for cross cluster generation
      --dnsentry-source.target-owner-id string                      owner id to use for generated DNS entries
      --dnsentry-source.target-realms string                        realm(s) to use for generated DNS entries
      --dnsentry-source.target-set-ignore-owners                    mark generated DNS entries to omit owner based access control
      --dnsentry-source.targets.pool.size int                       Worker pool size for pool targets of controller dnsentry-source (default: 2)
      --dry-run                                                     default for all controller "dry-run" options
      --exclude-domains stringArray                                 default for all controller "exclude-domains" options
      --google-clouddns.cache-dir string                            Directory to store zone caches (for reload after restart)
      --google-clouddns.cache-ttl int                               Time-to-live for provider hosted zone cache
      --google-clouddns.default.pool.size int                       Worker pool size for pool default of controller google-clouddns (default: 2)
      --google-clouddns.disable-zone-state-caching                  disable use of cached dns zone state on changes
      --google-clouddns.dns-class string                            Identifier used to differentiate responsible controllers for entries
      --google-clouddns.dns-delay duration                          delay between two dns reconciliations
      --google-clouddns.dns.pool.resync-period duration             Period for resynchronization of pool dns of controller google-clouddns (default: 15m0s)
      --google-clouddns.dns.pool.size int                           Worker pool size for pool dns of controller google-clouddns (default: 1)
      --google-clouddns.dry-run                                     just check, don't modify
      --google-clouddns.identifier string                           Identifier used to mark DNS entries
      --google-clouddns.ownerids.pool.size int                      Worker pool size for pool ownerids of controller google-clouddns (default: 1)
      --google-clouddns.providers.pool.resync-period duration       Period for resynchronization of pool providers of controller google-clouddns (default: 10m0s)
      --google-clouddns.providers.pool.size int                     Worker pool size for pool providers of controller google-clouddns (default: 2)
      --google-clouddns.reschedule-delay duration                   reschedule delay after losing provider
      --google-clouddns.secrets.pool.size int                       Worker pool size for pool secrets of controller google-clouddns (default: 2)
      --google-clouddns.setup int                                   number of processors for controller setup
      --google-clouddns.ttl int                                     Default time-to-live for DNS entries
      --grace-period duration                                       inactivity grace period for detecting end of cleanup for shutdown
  -h, --help                                                        help for dns-controller-manager
      --identifier string                                           default for all controller "identifier" options
      --ingress-dns.default.pool.resync-period duration             Period for resynchronization of pool default of controller ingress-dns (default: 2m0s)
      --ingress-dns.default.pool.size int                           Worker pool size for pool default of controller ingress-dns (default: 2)
      --ingress-dns.dns-class string                                identifier used to differentiate responsible controllers for entries
      --ingress-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries
      --ingress-dns.exclude-domains stringArray                     excluded domains
      --ingress-dns.key string                                      selecting key for annotation
      --ingress-dns.target-creator-label-name string                label name to store the creator for generated DNS entries
      --ingress-dns.target-creator-label-value string               label value for creator label
      --ingress-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation
      --ingress-dns.target-namespace string                         target namespace for cross cluster generation
      --ingress-dns.target-owner-id string                          owner id to use for generated DNS entries
      --ingress-dns.target-realms string                            realm(s) to use for generated DNS entries
      --ingress-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control
      --ingress-dns.targets.pool.size int                           Worker pool size for pool targets of controller ingress-dns (default: 2)
      --key string                                                  default for all controller "key" options
      --kubeconfig string                                           default cluster access
      --kubeconfig.disable-deploy-crds                              disable deployment of required crds for cluster default
      --kubeconfig.id string                                        id for cluster default
  -D, --log-level string                                            logrus log level
      --name string                                                 name used for controller manager
      --namespace string                                            namespace for lease
  -n, --namespace-local-access-only                                 enable access restriction for namespace local access only (deprecated)
      --omit-lease                                                  omit lease for development
      --openstack-designate.cache-dir string                        Directory to store zone caches (for reload after restart)
      --openstack-designate.cache-ttl int                           Time-to-live for provider hosted zone cache
      --openstack-designate.default.pool.size int                   Worker pool size for pool default of controller openstack-designate (default: 2)
      --openstack-designate.disable-zone-state-caching              disable use of cached dns zone state on changes
      --openstack-designate.dns-class string                        Identifier used to differentiate responsible controllers for entries
      --openstack-designate.dns-delay duration                      delay between two dns reconciliations
      --openstack-designate.dns.pool.resync-period duration         Period for resynchronization of pool dns of controller openstack-designate (default: 15m0s)
      --openstack-designate.dns.pool.size int                       Worker pool size for pool dns of controller openstack-designate (default: 1)
      --openstack-designate.dry-run                                 just check, don't modify
      --openstack-designate.identifier string                       Identifier used to mark DNS entries
      --openstack-designate.ownerids.pool.size int                  Worker pool size for pool ownerids of controller openstack-designate (default: 1)
      --openstack-designate.providers.pool.resync-period duration   Period for resynchronization of pool providers of controller openstack-designate (default: 10m0s)
      --openstack-designate.providers.pool.size int                 Worker pool size for pool providers of controller openstack-designate (default: 2)
      --openstack-designate.reschedule-delay duration               reschedule delay after losing provider
      --openstack-designate.secrets.pool.size int                   Worker pool size for pool secrets of controller openstack-designate (default: 2)
      --openstack-designate.setup int                               number of processors for controller setup
      --openstack-designate.ttl int                                 Default time-to-live for DNS entries
      --plugin-dir string                                           directory containing go plugins
      --pool.resync-period duration                                 default for all controller "pool.resync-period" options
      --pool.size int                                               default for all controller "pool.size" options
      --providers string                                            cluster to look for provider objects
      --providers.disable-deploy-crds                               disable deployment of required crds for cluster provider
      --providers.id string                                         id for cluster provider
      --reschedule-delay duration                                   default for all controller "reschedule-delay" options
      --server-port-http int                                        HTTP server port (serving /healthz, /metrics, ...)
      --service-dns.default.pool.resync-period duration             Period for resynchronization of pool default of controller service-dns (default: 2m0s)
      --service-dns.default.pool.size int                           Worker pool size for pool default of controller service-dns (default: 2)
      --service-dns.dns-class string                                identifier used to differentiate responsible controllers for entries
      --service-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries
      --service-dns.exclude-domains stringArray                     excluded domains
      --service-dns.key string                                      selecting key for annotation
      --service-dns.target-creator-label-name string                label name to store the creator for generated DNS entries
      --service-dns.target-creator-label-value string               label value for creator label
      --service-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation
      --service-dns.target-namespace string                         target namespace for cross cluster generation
      --service-dns.target-owner-id string                          owner id to use for generated DNS entries
      --service-dns.target-realms string                            realm(s) to use for generated DNS entries
      --service-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control
      --service-dns.targets.pool.size int                           Worker pool size for pool targets of controller service-dns (default: 2)
      --setup int                                                   default for all controller "setup" options
      --target string                                               target cluster for dns requests
      --target-creator-label-name string                            default for all controller "target-creator-label-name" options
      --target-creator-label-value string                           default for all controller "target-creator-label-value" options
      --target-name-prefix string                                   default for all controller "target-name-prefix" options
      --target-namespace string                                     default for all controller "target-namespace" options
      --target-owner-id string                                      default for all controller "target-owner-id" options
      --target-realms string                                        default for all controller "target-realms" options
      --target-set-ignore-owners                                    default for all controller "target-set-ignore-owners" options
      --target.disable-deploy-crds                                  disable deployment of required crds for cluster target
      --target.id string                                            id for cluster target
      --ttl int                                                     default for all controller "ttl" options
```

## Extensions

This project can also be used as library to implement own source and provisioning controllers.

### How to implement Source Controllers

Based on the provided source controller library a source controller must
implement the [`source.DNSSource` interface](pkg/dns/source/interface.go) and
provide an appropriate creator function.

A source controller can be implemented following this example:

```go
package service

import (
    "github.com/gardener/controller-manager-library/pkg/resources"
    "github.com/gardener/external-dns-management/pkg/dns/source"
)

var _MAIN_RESOURCE = resources.NewGroupKind("core", "Service")

func init() {
    source.DNSSourceController(source.NewDNSSouceTypeForExtractor("service-dns", _MAIN_RESOURCE, GetTargets),nil).
        FinalizerDomain("dns.gardener.cloud").
        MustRegister(source.CONTROLLER_GROUP_DNS_SOURCES)
}
```

Complete examples can be found in the sub packages of `pkg/controller/source`.

### How to implement Provisioning Controllers

Provisioning controllers can be implemented based on the provisioning controller library
in this repository and must implement the
[`provider.DNSHandlerFactory` interface](pkg/dns/provider/interface.go).
This factory returns implementations of the [`provider.DNSHandler` interface](pkg/dns/provider/interface.go)
that does the effective work for a dedicated set of hosted zones.

These factories can be embedded into a final controller manager (the runnable
instance) in several ways:

- The factory can be used to create a dedicated controller.
  This controller can then be embedded into a controller manager, either in
  its own controller manger or together with other controllers.
- The factory can be added to a compound factory, able to handle multiple
  infrastructures. This one can then be used to create a dedicated controller,
  again.

#### Embedding a Factory into a Controller

A provisioning controller can be implemented following this
[example](pkg/controller/provider/aws/controller/controller.go):

```go
package controller

import (
    "github.com/gardener/external-dns-management/pkg/dns/provider"
)

const CONTROLLER_NAME = "route53-dns-controller"

func init() {
    provider.DNSController(CONTROLLER_NAME, &Factory{}).
        FinalizerDomain("dns.gardener.cloud").
        MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}
```

This controller can be embedded into a controller manager just by using
an [anonymous import](cmd/dns/main.go) of the controller package in the main package
of a dedicated controller manager.

Complete examples are available in the sub packages of `pkg/controller/provider`.
They also show a typical set of implementation structures that help
to structure the implementation of such controllers.

The provider implemented in this project always follow the same structure:

- the provider package contains the provider code
- the factory source file registers the factory at a default compound factory
- it contains a sub package `controller`, which contains the embedding of
  the factory into a dedicated controller

#### Embedding a Factory into a Compound Factory

A provisioning controller based on a *Compound Factory* can be extended by
a new provider factory by registering this factory at the compound factory.
This could be done, for example,  by using the default compound factory provided
in package [`pkg/controller/provider/compound`](pkg/controller/provider/compound/factory.go) as shown
[here](pkg/controller/provider/aws/factory.go), where `NewHandler` is a function creating
a dedicated handler for a dedicated provider type:

```go

package aws

import (
    "github.com/gardener/external-dns-management/pkg/controller/provider/compound"
    "github.com/gardener/external-dns-management/pkg/dns/provider"
)

const TYPE_CODE = "aws-route53"

var Factory = provider.NewDNSHandlerFactory(TYPE_CODE, NewHandler)

func init() {
    compound.MustRegister(Factory)
}
```

The compound factory is then again embedded into a provisioning controller as shown
in the previous section (see the [`controller`sub package](pkg/controller/provider/compound/controller/controller.go)).

### Setting Up a Controller Manager

One or multiple controller packages can be bundled into a controller manager,
by implementing a main package like [this](cmd/dns/main.go):

```go
package main

import (
    "github.com/gardener/controller-manager-library/pkg/controllermanager"

    _ "github.com/<your controller package>"
    ...
)

func main() {
    controllermanager.Start("my-dns-controller-manager", "dns controller manager", "some description")
}
```

### Using the standard Compound Provisioning Controller

If the standard *Compound Provisioning Controller* should be used it is required
to additionally add the anonymous imports for the providers intended to be
embedded into the compound factory like [this](cmd/compound/main.go):

<details>
<summary><b>Example Coding</b></summary>

```go
package main

import (
    "fmt"
    "os"

    "github.com/gardener/controller-manager-library/pkg/controllermanager"


    _ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/<your provider>"
    ...
)

func main() {
    controllermanager.Start("dns-controller-manager", "dns controller manager", "nothing")
}
```

</details>

### Multiple Cluster Support

The controller implementations provided in this project are prepared to work
with multiple clusters by using the features of the used controller manager
library.

The *DNS Source Controllers* support two clusters:
- the default cluster is used to scan for source objects
- the logical cluster `target` is used to maintain the `DNSEnry` objects.

The *DNS Provisioning Controllers* also support two clusters:
- the default cluster is used to scan for `DNSEntry` objects. It is mapped
  to the logical cluster `target`
- the logical cluster `provider` is used to look to the `DNSProvider` objects
  and their related secrets.

If those controller types should be combined in a single controller manager,
it can be configured to support three potential clusters with the
source objects, the one for the entry objects and the one with provider
objects using cluster mappings.

This is shown in a complete [example](cmd/compound/main.go) using the dns
source controllers, the compound provisioning controller configured to
support all the included DNS provider type factories:

<details>
<summary><b>Example Coding</b></summary>

```go
package main

import (
    "fmt"
    "os"

    "github.com/gardener/controller-manager-library/pkg/controllermanager"
    "github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
    "github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
    "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"

    dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
    dnssource "github.com/gardener/external-dns-management/pkg/dns/source"

    _ "github.com/gardener/external-dns-management/pkg/controller/provider/compound/controller"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/alicloud"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/aws"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/azure"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/google"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/openstack"
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/cloudflare"

    _ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
    _ "github.com/gardener/external-dns-management/pkg/controller/source/service"
)

func init() {
    // target cluster already defined in dns source controller package
    cluster.Configure(
        dnsprovider.PROVIDER_CLUSTER,
        "providers",
        "cluster to look for provider objects",
    ).Fallback(dnssource.TARGET_CLUSTER)

    mappings.ForControllerGroup(dnsprovider.CONTROLLER_GROUP_DNS_CONTROLLERS).
        Map(controller.CLUSTER_MAIN, dnssource.TARGET_CLUSTER).MustRegister()

}

func main() {
    controllermanager.Start("dns-controller-manager", "dns controller manager", "nothing")
}
```

</details>

Those clusters can the be separated by registering their names together with
command line option names. These can be used to specify different kubeconfig
files for those clusters.

By default all logical clusters are mapped to the default physical cluster
specified via `--kubeconfig` or default cluster access.

If multiple physical clusters are defined they can be specified by a
corresponding cluster option defining the kubeconfig file used to access
this cluster. If no such option is specified the default is used.

Therefore, even if the configuration is prepared for multiple clusters,
such a controller manager can easily work on a single cluster if no special
options are given on the command line.

## Why not using the community external-dns solution?

Some of the reasons are context-specific, i.e. relate to Gardener's highly dynamic requirements.

1. Custom resource for DNS entries

DNS entries are explicitly specified as custom resources. As an important side effect, each DNS entry provides an own status. Simply by querying the Kubernetes API, a client can check if a requested DNS entry has been successfully added to the DNS backend, or if an update has already been deployed, or if not to reason about the cause. It also opens for easy extensibility, as DNS entries can be created directly via the Kubernetes API. And it simplifies Day 2 operations, e.g. automatic cleanup of unused entries if a DNS provider is deleted.

2. Management of multiple DNS providers

The Gardener DNS controller uses a custom resource DNSProvider to dynamically manage the backend DNS services. While with external-dns you have to specify the single provider during startup, in the Gardener DNS controller you can add/update/delete providers during runtime with different credentials and/or backends. This is important for a multi-tenant environment as in Gardener, where users can bring their own accounts.

A DNS provider can also restrict its actions on subset of the DNS domains (includes and excludes) for which the credentials are capable to edit.

Each provider can define a separate “owner” identifier, to differentiate DNS entries in the same DNS zone from different providers.

3. Multi cluster support

The Gardener DNS controller distinguish three different logical Kubernetes clusters: Source cluster, target cluster and runtime cluster. The source cluster is monitored by the DNS source controllers for annotations on ingress and service resources. These controllers then create DNS entries in the target cluster. DNS entries in the target cluster are then reconciliated/synchronized with the corresponding DNS backend service by the provider controller. The runtime cluster is the cluster the DNS controller runs on. For example, this enables needed flexibility in the Gardener deployment. The DNS controller runs on the seed cluster. This is also the target cluster. DNS providers and entries resources are created in the corresponding namespace of the shoot control plane, while the source cluster is the shoot cluster itself.

4. Optimizations for handling hundreds of DNS entries

Some DNS backend services are restricted on the API calls per second (e.g. the AWS Route 53 API). To manage hundreds of DNS entries it is important to minimize the number of API calls. The Gardener DNS controller heavily makes usage of caches and batch processing for this reason.
