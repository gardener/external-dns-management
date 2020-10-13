# External DNS Management

The main artefact of this project is the <b>DNS controller manager</b> for managing DNS records, also
nicknamed as the Gardener "DNS Controller".

It contains provisioning controllers for creating DNS records in one of the DNS cloud services
  - [_Amazon Route53_](/docs/aws-route53/README.md),
  - _Google CloudDNS_,
  - _AliCloud DNS_,
  - _Azure DNS_,
  - _OpenStack Designate_,
  - [_Cloudflare DNS_](/docs/cloudflare/README.md),
  - [_Infoblox_](/docs/infoblox/README.md),

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

### Automatic creation of DNS entries for services and ingresses

Using the source controllers, it is also possible to create DNS entries for services (of type `LoadBalancer`)
and ingresses automatically. The resources only need to be annotated with some special values.
In this case ensure that the source controllers are enabled on startup of the DNS controller manager, i.e. the
value of the command line option `--controllers` must contain `dnscontrollers` or equal to `all`.
The DNS source controllers watch resources on the default cluster and create DNS entries on
the target cluster. As there can be multiple controllers active on the same cluster, you may
need to set the correct `DNSClass` both for the controller and for the source resource by
setting the annotation `dns.gardener.cloud/class`. The default value for the `DNSClass` is `gardendns`.

Note that if you delegate the DNS management for shoot resources to Gardener via the 
[shoot-dns-service extension](https://github.com/gardener/gardener-extension-shoot-dns-service),
the correct annotation is `dns.gardener.cloud/class=garden`.

Here is an example for annotating a service (same as `examples/50-service-with-dns.yaml`)]:

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: echo.my-dns-domain.com
    dns.gardener.cloud/ttl: "500"
    # If you are delegating the DNS Management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/dns_names/)
    #`dns.gardener.cloud/class`: garden
  name: test-service
  namespace: default
spec:
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 8080
  sessionAffinity: None
  type: LoadBalancer
``` 

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

![Model Overview](docs/model.png)

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

### DNSAnnotation objects

DNS source controllers support the creation of DNS entries for potentiialy
any kind of resource originally not equipped to describe the generation of
DNS entries. This is done by additionally annotations. Nevertheless it
might be the case, that those objects are again the result of a generation
process, ether by predefined helm starts or by other higher level controllers.
It is not necessarily possible to influence those generation steps to
additionally generate the deired DNS annotations. 

The typical mechanis in Kubernetes to handle this is to provide mutating
webhooks that enrich the generated objects accordingly. But this mechanism
is basically not intended to support dedicated settings for dedicated instances.
At least it is very strenous to provide web hooks for every such usecase.

Therefore the DNS ecosystem provided by this project supports an additional
extension mechanism to annotate any kind of object with additional annotations
by supported a dedicated resource, the `DNSAnnotation`. 

The handling of this resource is done by a dedicated controller, the `annotation`
controller. It caches the annotation settings declared by those objects and
makes them accessible for the DNS source controllers.

The DNS source controller responsible for a dedicated kind of resource
(for example Service reads the object analyses the annotations and then decides
what to do with it. Most of the flow is handled by a central library, only
some dedicated resource dependent steps are implemented separately by a
dedicated source controller. The `DNSAnnotation`resource slightly extends this
flow: After reading the object the library additionally checks for the existence
of a `DNSAnnotation` setting for this object by querying the `annotation`
controller's cache. If found, it adds annotations declared there to the original
object prior to the next processing steps.
This way, for example whenver a `Service` without
any DNS related annotation is handled by the controller and it finds a matching
`DNSAnnotation` setting, the set of actual annotations is enriched accordingly
before the actual processing of the service object is done by the controller.

This `DNSAnnotation` object can be created before or even after the object to
be annotated and will implicity cause a reprocessing of the original object by
its DNS source controller.

For example, the following object enforces a DNS related annotation for the
processing of the service object `testapp/default` by the service DNS source 
controller:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSAnnotation
metadata:
  name: testapp
spec:
  resourceRef:
    kind: Service
    apiVersion: v1
    name: testapp
  annotations:
    dns.gardener.cloud/dnsnames: testapp.dns.gardener.cloud
    dns.gardener.cloud/ttl: "500"
```

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
      --accepted-maintainers string                                 accepted maintainer key(s) for crds
      --alicloud-dns.cache-dir string                               Directory to store zone caches (for reload after restart) of controller alicloud-dns
      --alicloud-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache of controller alicloud-dns (default 120)
      --alicloud-dns.default.pool.size int                          Worker pool size for pool default of controller alicloud-dns (default 2)
      --alicloud-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes of controller alicloud-dns
      --alicloud-dns.dns-class string                               Class identifier used to differentiate responsible controllers for entry resources of controller alicloud-dns (default "gardendns")
      --alicloud-dns.dns-delay duration                             delay between two dns reconciliations of controller alicloud-dns (default 10s)
      --alicloud-dns.dns.pool.resync-period duration                Period for resynchronization for pool dns of controller alicloud-dns (default 15m0s)
      --alicloud-dns.dns.pool.size int                              Worker pool size for pool dns of controller alicloud-dns (default 1)
      --alicloud-dns.dry-run                                        just check, don't modify of controller alicloud-dns
      --alicloud-dns.identifier string                              Identifier used to mark DNS entries in DNS system of controller alicloud-dns (default "dnscontroller")
      --alicloud-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller alicloud-dns (default 1)
      --alicloud-dns.pool.resync-period duration                    Period for resynchronization of controller alicloud-dns
      --alicloud-dns.pool.size int                                  Worker pool size of controller alicloud-dns
      --alicloud-dns.providers.pool.resync-period duration          Period for resynchronization for pool providers of controller alicloud-dns (default 10m0s)
      --alicloud-dns.providers.pool.size int                        Worker pool size for pool providers of controller alicloud-dns (default 2)
      --alicloud-dns.ratelimiter.burst int                          number of burst requests for rate limiter of controller alicloud-dns
      --alicloud-dns.ratelimiter.enabled                            enables rate limiter for DNS provider requests of controller alicloud-dns
      --alicloud-dns.ratelimiter.qps int                            maximum requests/queries per second of controller alicloud-dns
      --alicloud-dns.reschedule-delay duration                      reschedule delay after losing provider of controller alicloud-dns (default 2m0s)
      --alicloud-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller alicloud-dns (default 2)
      --alicloud-dns.setup int                                      number of processors for controller setup of controller alicloud-dns (default 10)
      --alicloud-dns.statistic.pool.size int                        Worker pool size for pool statistic of controller alicloud-dns (default 1)
      --alicloud-dns.ttl int                                        Default time-to-live for DNS entries of controller alicloud-dns (default 300)
      --annotation.default.pool.size int                            Worker pool size for pool default of controller annotation (default 5)
      --annotation.pool.size int                                    Worker pool size of controller annotation
      --annotation.setup int                                        number of processors for controller setup of controller annotation (default 10)
      --aws-route53.cache-dir string                                Directory to store zone caches (for reload after restart) of controller aws-route53
      --aws-route53.cache-ttl int                                   Time-to-live for provider hosted zone cache of controller aws-route53 (default 120)
      --aws-route53.default.pool.size int                           Worker pool size for pool default of controller aws-route53 (default 2)
      --aws-route53.disable-zone-state-caching                      disable use of cached dns zone state on changes of controller aws-route53
      --aws-route53.dns-class string                                Class identifier used to differentiate responsible controllers for entry resources of controller aws-route53 (default "gardendns")
      --aws-route53.dns-delay duration                              delay between two dns reconciliations of controller aws-route53 (default 10s)
      --aws-route53.dns.pool.resync-period duration                 Period for resynchronization for pool dns of controller aws-route53 (default 15m0s)
      --aws-route53.dns.pool.size int                               Worker pool size for pool dns of controller aws-route53 (default 1)
      --aws-route53.dry-run                                         just check, don't modify of controller aws-route53
      --aws-route53.identifier string                               Identifier used to mark DNS entries in DNS system of controller aws-route53 (default "dnscontroller")
      --aws-route53.ownerids.pool.size int                          Worker pool size for pool ownerids of controller aws-route53 (default 1)
      --aws-route53.pool.resync-period duration                     Period for resynchronization of controller aws-route53
      --aws-route53.pool.size int                                   Worker pool size of controller aws-route53
      --aws-route53.providers.pool.resync-period duration           Period for resynchronization for pool providers of controller aws-route53 (default 10m0s)
      --aws-route53.providers.pool.size int                         Worker pool size for pool providers of controller aws-route53 (default 2)
      --aws-route53.ratelimiter.burst int                           number of burst requests for rate limiter of controller aws-route53
      --aws-route53.ratelimiter.enabled                             enables rate limiter for DNS provider requests of controller aws-route53
      --aws-route53.ratelimiter.qps int                             maximum requests/queries per second of controller aws-route53
      --aws-route53.reschedule-delay duration                       reschedule delay after losing provider of controller aws-route53 (default 2m0s)
      --aws-route53.secrets.pool.size int                           Worker pool size for pool secrets of controller aws-route53 (default 2)
      --aws-route53.setup int                                       number of processors for controller setup of controller aws-route53 (default 10)
      --aws-route53.statistic.pool.size int                         Worker pool size for pool statistic of controller aws-route53 (default 1)
      --aws-route53.ttl int                                         Default time-to-live for DNS entries of controller aws-route53 (default 300)
      --azure-dns.cache-dir string                                  Directory to store zone caches (for reload after restart) of controller azure-dns
      --azure-dns.cache-ttl int                                     Time-to-live for provider hosted zone cache of controller azure-dns (default 120)
      --azure-dns.default.pool.size int                             Worker pool size for pool default of controller azure-dns (default 2)
      --azure-dns.disable-zone-state-caching                        disable use of cached dns zone state on changes of controller azure-dns
      --azure-dns.dns-class string                                  Class identifier used to differentiate responsible controllers for entry resources of controller azure-dns (default "gardendns")
      --azure-dns.dns-delay duration                                delay between two dns reconciliations of controller azure-dns (default 10s)
      --azure-dns.dns.pool.resync-period duration                   Period for resynchronization for pool dns of controller azure-dns (default 15m0s)
      --azure-dns.dns.pool.size int                                 Worker pool size for pool dns of controller azure-dns (default 1)
      --azure-dns.dry-run                                           just check, don't modify of controller azure-dns
      --azure-dns.identifier string                                 Identifier used to mark DNS entries in DNS system of controller azure-dns (default "dnscontroller")
      --azure-dns.ownerids.pool.size int                            Worker pool size for pool ownerids of controller azure-dns (default 1)
      --azure-dns.pool.resync-period duration                       Period for resynchronization of controller azure-dns
      --azure-dns.pool.size int                                     Worker pool size of controller azure-dns
      --azure-dns.providers.pool.resync-period duration             Period for resynchronization for pool providers of controller azure-dns (default 10m0s)
      --azure-dns.providers.pool.size int                           Worker pool size for pool providers of controller azure-dns (default 2)
      --azure-dns.ratelimiter.burst int                             number of burst requests for rate limiter of controller azure-dns
      --azure-dns.ratelimiter.enabled                               enables rate limiter for DNS provider requests of controller azure-dns
      --azure-dns.ratelimiter.qps int                               maximum requests/queries per second of controller azure-dns
      --azure-dns.reschedule-delay duration                         reschedule delay after losing provider of controller azure-dns (default 2m0s)
      --azure-dns.secrets.pool.size int                             Worker pool size for pool secrets of controller azure-dns (default 2)
      --azure-dns.setup int                                         number of processors for controller setup of controller azure-dns (default 10)
      --azure-dns.statistic.pool.size int                           Worker pool size for pool statistic of controller azure-dns (default 1)
      --azure-dns.ttl int                                           Default time-to-live for DNS entries of controller azure-dns (default 300)
      --bind-address-http string                                    HTTP server bind address
      --cache-dir string                                            Directory to store zone caches (for reload after restart)
      --cache-ttl int                                               Time-to-live for provider hosted zone cache
      --cloudflare-dns.cache-dir string                             Directory to store zone caches (for reload after restart) of controller cloudflare-dns
      --cloudflare-dns.cache-ttl int                                Time-to-live for provider hosted zone cache of controller cloudflare-dns (default 120)
      --cloudflare-dns.default.pool.size int                        Worker pool size for pool default of controller cloudflare-dns (default 2)
      --cloudflare-dns.disable-zone-state-caching                   disable use of cached dns zone state on changes of controller cloudflare-dns
      --cloudflare-dns.dns-class string                             Class identifier used to differentiate responsible controllers for entry resources of controller cloudflare-dns (default "gardendns")
      --cloudflare-dns.dns-delay duration                           delay between two dns reconciliations of controller cloudflare-dns (default 10s)
      --cloudflare-dns.dns.pool.resync-period duration              Period for resynchronization for pool dns of controller cloudflare-dns (default 15m0s)
      --cloudflare-dns.dns.pool.size int                            Worker pool size for pool dns of controller cloudflare-dns (default 1)
      --cloudflare-dns.dry-run                                      just check, don't modify of controller cloudflare-dns
      --cloudflare-dns.identifier string                            Identifier used to mark DNS entries in DNS system of controller cloudflare-dns (default "dnscontroller")
      --cloudflare-dns.ownerids.pool.size int                       Worker pool size for pool ownerids of controller cloudflare-dns (default 1)
      --cloudflare-dns.pool.resync-period duration                  Period for resynchronization of controller cloudflare-dns
      --cloudflare-dns.pool.size int                                Worker pool size of controller cloudflare-dns
      --cloudflare-dns.providers.pool.resync-period duration        Period for resynchronization for pool providers of controller cloudflare-dns (default 10m0s)
      --cloudflare-dns.providers.pool.size int                      Worker pool size for pool providers of controller cloudflare-dns (default 2)
      --cloudflare-dns.ratelimiter.burst int                        number of burst requests for rate limiter of controller cloudflare-dns
      --cloudflare-dns.ratelimiter.enabled                          enables rate limiter for DNS provider requests of controller cloudflare-dns
      --cloudflare-dns.ratelimiter.qps int                          maximum requests/queries per second of controller cloudflare-dns
      --cloudflare-dns.reschedule-delay duration                    reschedule delay after losing provider of controller cloudflare-dns (default 2m0s)
      --cloudflare-dns.secrets.pool.size int                        Worker pool size for pool secrets of controller cloudflare-dns (default 2)
      --cloudflare-dns.setup int                                    number of processors for controller setup of controller cloudflare-dns (default 10)
      --cloudflare-dns.statistic.pool.size int                      Worker pool size for pool statistic of controller cloudflare-dns (default 1)
      --cloudflare-dns.ttl int                                      Default time-to-live for DNS entries of controller cloudflare-dns (default 300)
      --config string                                               config file
  -c, --controllers string                                          comma separated list of controllers to start (<name>,<group>,all) (default "all")
      --cpuprofile string                                           set file for cpu profiling
      --default.pool.resync-period duration                         Period for resynchronization for pool default
      --default.pool.size int                                       Worker pool size for pool default
      --disable-namespace-restriction                               disable access restriction for namespace local access only
      --disable-zone-state-caching                                  disable use of cached dns zone state on changes
      --dns-class string                                            identifier used to differentiate responsible controllers for entries
      --dns-delay duration                                          delay between two dns reconciliations
      --dns-target-class string                                     identifier used to differentiate responsible dns controllers for target entries
      --dns.pool.resync-period duration                             Period for resynchronization for pool dns
      --dns.pool.size int                                           Worker pool size for pool dns
      --dnsentry-source.default.pool.resync-period duration         Period for resynchronization for pool default of controller dnsentry-source (default 2m0s)
      --dnsentry-source.default.pool.size int                       Worker pool size for pool default of controller dnsentry-source (default 2)
      --dnsentry-source.dns-class string                            identifier used to differentiate responsible controllers for entries of controller dnsentry-source (default "gardendns")
      --dnsentry-source.dns-target-class string                     identifier used to differentiate responsible dns controllers for target entries of controller dnsentry-source
      --dnsentry-source.exclude-domains stringArray                 excluded domains of controller dnsentry-source
      --dnsentry-source.key string                                  selecting key for annotation of controller dnsentry-source
      --dnsentry-source.pool.resync-period duration                 Period for resynchronization of controller dnsentry-source
      --dnsentry-source.pool.size int                               Worker pool size of controller dnsentry-source
      --dnsentry-source.target-creator-label-name string            label name to store the creator for generated DNS entries of controller dnsentry-source (default "creator")
      --dnsentry-source.target-creator-label-value string           label value for creator label of controller dnsentry-source
      --dnsentry-source.target-name-prefix string                   name prefix in target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-namespace string                     target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-owner-id string                      owner id to use for generated DNS entries of controller dnsentry-source
      --dnsentry-source.target-realms string                        realm(s) to use for generated DNS entries of controller dnsentry-source
      --dnsentry-source.target-set-ignore-owners                    mark generated DNS entries to omit owner based access control of controller dnsentry-source
      --dnsentry-source.targets.pool.size int                       Worker pool size for pool targets of controller dnsentry-source (default 2)
      --dry-run                                                     just check, don't modify
      --exclude-domains stringArray                                 excluded domains
      --force-crd-update                                            enforce update of crds even they are unmanaged
      --google-clouddns.cache-dir string                            Directory to store zone caches (for reload after restart) of controller google-clouddns
      --google-clouddns.cache-ttl int                               Time-to-live for provider hosted zone cache of controller google-clouddns (default 120)
      --google-clouddns.default.pool.size int                       Worker pool size for pool default of controller google-clouddns (default 2)
      --google-clouddns.disable-zone-state-caching                  disable use of cached dns zone state on changes of controller google-clouddns
      --google-clouddns.dns-class string                            Class identifier used to differentiate responsible controllers for entry resources of controller google-clouddns (default "gardendns")
      --google-clouddns.dns-delay duration                          delay between two dns reconciliations of controller google-clouddns (default 10s)
      --google-clouddns.dns.pool.resync-period duration             Period for resynchronization for pool dns of controller google-clouddns (default 15m0s)
      --google-clouddns.dns.pool.size int                           Worker pool size for pool dns of controller google-clouddns (default 1)
      --google-clouddns.dry-run                                     just check, don't modify of controller google-clouddns
      --google-clouddns.identifier string                           Identifier used to mark DNS entries in DNS system of controller google-clouddns (default "dnscontroller")
      --google-clouddns.ownerids.pool.size int                      Worker pool size for pool ownerids of controller google-clouddns (default 1)
      --google-clouddns.pool.resync-period duration                 Period for resynchronization of controller google-clouddns
      --google-clouddns.pool.size int                               Worker pool size of controller google-clouddns
      --google-clouddns.providers.pool.resync-period duration       Period for resynchronization for pool providers of controller google-clouddns (default 10m0s)
      --google-clouddns.providers.pool.size int                     Worker pool size for pool providers of controller google-clouddns (default 2)
      --google-clouddns.ratelimiter.burst int                       number of burst requests for rate limiter of controller google-clouddns
      --google-clouddns.ratelimiter.enabled                         enables rate limiter for DNS provider requests of controller google-clouddns
      --google-clouddns.ratelimiter.qps int                         maximum requests/queries per second of controller google-clouddns
      --google-clouddns.reschedule-delay duration                   reschedule delay after losing provider of controller google-clouddns (default 2m0s)
      --google-clouddns.secrets.pool.size int                       Worker pool size for pool secrets of controller google-clouddns (default 2)
      --google-clouddns.setup int                                   number of processors for controller setup of controller google-clouddns (default 10)
      --google-clouddns.statistic.pool.size int                     Worker pool size for pool statistic of controller google-clouddns (default 1)
      --google-clouddns.ttl int                                     Default time-to-live for DNS entries of controller google-clouddns (default 300)
      --grace-period duration                                       inactivity grace period for detecting end of cleanup for shutdown
  -h, --help                                                        help for dns-controller-manager
      --identifier string                                           Identifier used to mark DNS entries in DNS system
      --infoblox-dns.cache-dir string                               Directory to store zone caches (for reload after restart) of controller infoblox-dns
      --infoblox-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache of controller infoblox-dns (default 120)
      --infoblox-dns.default.pool.size int                          Worker pool size for pool default of controller infoblox-dns (default 2)
      --infoblox-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes of controller infoblox-dns
      --infoblox-dns.dns-class string                               Class identifier used to differentiate responsible controllers for entry resources of controller infoblox-dns (default "gardendns")
      --infoblox-dns.dns-delay duration                             delay between two dns reconciliations of controller infoblox-dns (default 10s)
      --infoblox-dns.dns.pool.resync-period duration                Period for resynchronization for pool dns of controller infoblox-dns (default 15m0s)
      --infoblox-dns.dns.pool.size int                              Worker pool size for pool dns of controller infoblox-dns (default 1)
      --infoblox-dns.dry-run                                        just check, don't modify of controller infoblox-dns
      --infoblox-dns.identifier string                              Identifier used to mark DNS entries in DNS system of controller infoblox-dns (default "dnscontroller")
      --infoblox-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller infoblox-dns (default 1)
      --infoblox-dns.pool.resync-period duration                    Period for resynchronization of controller infoblox-dns
      --infoblox-dns.pool.size int                                  Worker pool size of controller infoblox-dns
      --infoblox-dns.providers.pool.resync-period duration          Period for resynchronization for pool providers of controller infoblox-dns (default 10m0s)
      --infoblox-dns.providers.pool.size int                        Worker pool size for pool providers of controller infoblox-dns (default 2)
      --infoblox-dns.ratelimiter.burst int                          number of burst requests for rate limiter of controller infoblox-dns
      --infoblox-dns.ratelimiter.enabled                            enables rate limiter for DNS provider requests of controller infoblox-dns
      --infoblox-dns.ratelimiter.qps int                            maximum requests/queries per second of controller infoblox-dns
      --infoblox-dns.reschedule-delay duration                      reschedule delay after losing provider of controller infoblox-dns (default 2m0s)
      --infoblox-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller infoblox-dns (default 2)
      --infoblox-dns.setup int                                      number of processors for controller setup of controller infoblox-dns (default 10)
      --infoblox-dns.statistic.pool.size int                        Worker pool size for pool statistic of controller infoblox-dns (default 1)
      --infoblox-dns.ttl int                                        Default time-to-live for DNS entries of controller infoblox-dns (default 300)
      --ingress-dns.default.pool.resync-period duration             Period for resynchronization for pool default of controller ingress-dns (default 2m0s)
      --ingress-dns.default.pool.size int                           Worker pool size for pool default of controller ingress-dns (default 2)
      --ingress-dns.dns-class string                                identifier used to differentiate responsible controllers for entries of controller ingress-dns (default "gardendns")
      --ingress-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries of controller ingress-dns
      --ingress-dns.exclude-domains stringArray                     excluded domains of controller ingress-dns
      --ingress-dns.key string                                      selecting key for annotation of controller ingress-dns
      --ingress-dns.pool.resync-period duration                     Period for resynchronization of controller ingress-dns
      --ingress-dns.pool.size int                                   Worker pool size of controller ingress-dns
      --ingress-dns.target-creator-label-name string                label name to store the creator for generated DNS entries of controller ingress-dns (default "creator")
      --ingress-dns.target-creator-label-value string               label value for creator label of controller ingress-dns
      --ingress-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-namespace string                         target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-owner-id string                          owner id to use for generated DNS entries of controller ingress-dns
      --ingress-dns.target-realms string                            realm(s) to use for generated DNS entries of controller ingress-dns
      --ingress-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control of controller ingress-dns
      --ingress-dns.targets.pool.size int                           Worker pool size for pool targets of controller ingress-dns (default 2)
      --key string                                                  selecting key for annotation
      --kubeconfig string                                           default cluster access
      --kubeconfig.disable-deploy-crds                              disable deployment of required crds for cluster default
      --kubeconfig.id string                                        id for cluster default
      --kubeconfig.migration-ids string                             migration id for cluster default
      --lease-duration duration                                     lease duration (default 15s)
      --lease-name string                                           name for lease object
      --lease-renew-deadline duration                               lease renew deadline (default 10s)
      --lease-retry-period duration                                 lease retry period (default 2s)
  -D, --log-level string                                            logrus log level
      --maintainer string                                           maintainer key for crds (default "dns-controller-manager")
      --name string                                                 name used for controller manager (default "dns-controller-manager")
      --namespace string                                            namespace for lease (default "kube-system")
  -n, --namespace-local-access-only                                 enable access restriction for namespace local access only (deprecated)
      --omit-lease                                                  omit lease for development
      --openstack-designate.cache-dir string                        Directory to store zone caches (for reload after restart) of controller openstack-designate
      --openstack-designate.cache-ttl int                           Time-to-live for provider hosted zone cache of controller openstack-designate (default 120)
      --openstack-designate.default.pool.size int                   Worker pool size for pool default of controller openstack-designate (default 2)
      --openstack-designate.disable-zone-state-caching              disable use of cached dns zone state on changes of controller openstack-designate
      --openstack-designate.dns-class string                        Class identifier used to differentiate responsible controllers for entry resources of controller openstack-designate (default "gardendns")
      --openstack-designate.dns-delay duration                      delay between two dns reconciliations of controller openstack-designate (default 10s)
      --openstack-designate.dns.pool.resync-period duration         Period for resynchronization for pool dns of controller openstack-designate (default 15m0s)
      --openstack-designate.dns.pool.size int                       Worker pool size for pool dns of controller openstack-designate (default 1)
      --openstack-designate.dry-run                                 just check, don't modify of controller openstack-designate
      --openstack-designate.identifier string                       Identifier used to mark DNS entries in DNS system of controller openstack-designate (default "dnscontroller")
      --openstack-designate.ownerids.pool.size int                  Worker pool size for pool ownerids of controller openstack-designate (default 1)
      --openstack-designate.pool.resync-period duration             Period for resynchronization of controller openstack-designate
      --openstack-designate.pool.size int                           Worker pool size of controller openstack-designate
      --openstack-designate.providers.pool.resync-period duration   Period for resynchronization for pool providers of controller openstack-designate (default 10m0s)
      --openstack-designate.providers.pool.size int                 Worker pool size for pool providers of controller openstack-designate (default 2)
      --openstack-designate.ratelimiter.burst int                   number of burst requests for rate limiter of controller openstack-designate
      --openstack-designate.ratelimiter.enabled                     enables rate limiter for DNS provider requests of controller openstack-designate
      --openstack-designate.ratelimiter.qps int                     maximum requests/queries per second of controller openstack-designate
      --openstack-designate.reschedule-delay duration               reschedule delay after losing provider of controller openstack-designate (default 2m0s)
      --openstack-designate.secrets.pool.size int                   Worker pool size for pool secrets of controller openstack-designate (default 2)
      --openstack-designate.setup int                               number of processors for controller setup of controller openstack-designate (default 10)
      --openstack-designate.statistic.pool.size int                 Worker pool size for pool statistic of controller openstack-designate (default 1)
      --openstack-designate.ttl int                                 Default time-to-live for DNS entries of controller openstack-designate (default 300)
      --ownerids.pool.size int                                      Worker pool size for pool ownerids
      --plugin-file string                                          directory containing go plugins
      --pool.resync-period duration                                 Period for resynchronization
      --pool.size int                                               Worker pool size
      --providers string                                            cluster to look for provider objects
      --providers.disable-deploy-crds                               disable deployment of required crds for cluster provider
      --providers.id string                                         id for cluster provider
      --providers.migration-ids string                              migration id for cluster provider
      --providers.pool.resync-period duration                       Period for resynchronization for pool providers
      --providers.pool.size int                                     Worker pool size for pool providers
      --ratelimiter.burst int                                       number of burst requests for rate limiter
      --ratelimiter.enabled                                         enables rate limiter for DNS provider requests
      --ratelimiter.qps int                                         maximum requests/queries per second
      --reschedule-delay duration                                   reschedule delay after losing provider
      --secrets.pool.size int                                       Worker pool size for pool secrets
      --server-port-http int                                        HTTP server port (serving /healthz, /metrics, ...)
      --service-dns.default.pool.resync-period duration             Period for resynchronization for pool default of controller service-dns (default 2m0s)
      --service-dns.default.pool.size int                           Worker pool size for pool default of controller service-dns (default 2)
      --service-dns.dns-class string                                identifier used to differentiate responsible controllers for entries of controller service-dns (default "gardendns")
      --service-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries of controller service-dns
      --service-dns.exclude-domains stringArray                     excluded domains of controller service-dns
      --service-dns.key string                                      selecting key for annotation of controller service-dns
      --service-dns.pool.resync-period duration                     Period for resynchronization of controller service-dns
      --service-dns.pool.size int                                   Worker pool size of controller service-dns
      --service-dns.target-creator-label-name string                label name to store the creator for generated DNS entries of controller service-dns (default "creator")
      --service-dns.target-creator-label-value string               label value for creator label of controller service-dns
      --service-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation of controller service-dns
      --service-dns.target-namespace string                         target namespace for cross cluster generation of controller service-dns
      --service-dns.target-owner-id string                          owner id to use for generated DNS entries of controller service-dns
      --service-dns.target-realms string                            realm(s) to use for generated DNS entries of controller service-dns
      --service-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control of controller service-dns
      --service-dns.targets.pool.size int                           Worker pool size for pool targets of controller service-dns (default 2)
      --setup int                                                   number of processors for controller setup
      --statistic.pool.size int                                     Worker pool size for pool statistic
      --target string                                               target cluster for dns requests
      --target-creator-label-name string                            label name to store the creator for generated DNS entries
      --target-creator-label-value string                           label value for creator label
      --target-name-prefix string                                   name prefix in target namespace for cross cluster generation
      --target-namespace string                                     target namespace for cross cluster generation
      --target-owner-id string                                      owner id to use for generated DNS entries
      --target-realms string                                        realm(s) to use for generated DNS entries
      --target-set-ignore-owners                                    mark generated DNS entries to omit owner based access control
      --target.disable-deploy-crds                                  disable deployment of required crds for cluster target
      --target.id string                                            id for cluster target
      --target.migration-ids string                                 migration id for cluster target
      --targets.pool.size int                                       Worker pool size for pool targets
      --ttl int                                                     Default time-to-live for DNS entries
      --version                                                     version for dns-controller-manager
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
