# External DNS Management

[![REUSE status](https://api.reuse.software/badge/github.com/gardener/external-dns-management)](https://api.reuse.software/info/github.com/gardener/external-dns-management)

The main artefact of this project is the **DNS controller manager** for managing DNS records, also
nicknamed as the Gardener "DNS Controller".

It contains provisioning controllers for creating DNS records in one of the DNS cloud services
  - [_Amazon Route53_](/docs/aws-route53/README.md),
  - [_Google CloudDNS_](/docs/google-cloud-dns/README.md),
  - [_AliCloud DNS_](/docs/alicloud-dns/README.md),
  - [_Azure DNS_](/docs/azure-dns/README.md) and [_Azure Private_DNS_](/docs/azure-private-dns/README.md),
  - [_OpenStack Designate_](/docs/openstack-designate/README.md),
  - [_Cloudflare DNS_](/docs/cloudflare/README.md),
  - [_Infoblox_](/docs/infoblox/README.md),
  - [_Netlify DNS_](docs/netlify/README.md),
  - [_remote_](docs/remote/README.md),
  - [_DNS servers supporting RFC 2136 (DNS Update)_](docs/rfc2136/README.md) *(alpha - not recommended for productive usage)*,
  - [_powerdns_](docs/powerdns/README.md),

and source controllers for services and ingresses to create DNS entries by annotations.

The configuration for the external DNS service is specified in a custom resource `DNSProvider`.
Multiple `DNSProvider` can be used simultaneously and changed without restarting the DNS controller.

DNS records are either created directly for a corresponding custom resource `DNSEntry` or by
annotating a service or ingress.

For a detailed explanation of the model, see section [The Model](#the-model).

For extending or adapting this project with your own source or provisioning controllers, see section
[Extensions](#extensions)

## Index

- [External DNS Management](#external-dns-management)
  - [Index](#index)
  - [Important Note: Support for owner identifiers is discontinued](#important-note-support-for-owner-identifiers-is-discontinued)
  - [Quick start](#quick-start)
    - [Automatic creation of DNS entries for services and ingresses](#automatic-creation-of-dns-entries-for-services-and-ingresses)
      - [`A` DNS records with alias targets for provider type AWS-Route53 and AWS load balancers](#a-dns-records-with-alias-targets-for-provider-type-aws-route53-and-aws-load-balancers)
    - [Automatic creation of DNS entries for gateways](#automatic-creation-of-dns-entries-for-gateways)
      - [Istio gateways](#istio-gateways)
      - [Gateway API gateways](#gateway-api-gateways)
  - [The Model](#the-model)
    - [DNS Classes](#dns-classes)
    - [DNSAnnotation objects](#dnsannotation-objects)
  - [Using the DNS controller manager](#using-the-dns-controller-manager)
  - [Extensions](#extensions)
    - [How to implement Source Controllers](#how-to-implement-source-controllers)
    - [How to implement Provisioning Controllers](#how-to-implement-provisioning-controllers)
      - [Embedding a Factory into a Controller](#embedding-a-factory-into-a-controller)
      - [Embedding a Factory into a Compound Factory](#embedding-a-factory-into-a-compound-factory)
    - [Setting Up a Controller Manager](#setting-up-a-controller-manager)
    - [Using the standard Compound Provisioning Controller](#using-the-standard-compound-provisioning-controller)
    - [Multiple Cluster Support](#multiple-cluster-support)
  - [Why not use the community `external-dns` solution?](#why-not-use-the-community-external-dns-solution)

## Important Note: Support for owner identifiers is discontinued

Starting with release `v0.23`, the support for owner identifiers is discontinued.
With release `v0.24`, the support for owner identifiers is removed completely.
The custom resource `DNSOwner` are not supported anymore.

## Quick start

To install the **DNS controller manager** in your Kubernetes cluster, follow these steps.

1. Prerequisites
    - Check out or download the project to get a copy of the Helm charts.
      It is recommended to check out the tag of the
      [last release](https://github.com/gardener/external-dns-management/releases), so that Helm
      values reference the newest released container image for the deployment.

    - Make sure, that you have installed Helm client (`helm`) locally. See e.g. [Helm installation](https://helm.sh/docs/install/) for more details.

2. Install the DNS controller manager

    Then install the DNS controller manager with

    ```bash
    helm install dns-controller charts/external-dns-management --namespace=<my-namespace>
    ```

    This will use the default configuration with all source and provisioning controllers enabled.
    The complete set of configuration variables can be found in `charts/external-dns-management/values.yaml`.
    Their meaning is explained by their corresponding command line options in section
    [Using the DNS controller manager](#using-the-dns-controller-manager)

    By default, the DNS controller looks for custom resources in all namespaces. The chosen namespace is
    only relevant for the deployment itself.

    You may need to install [VerticalPodAutoscaler CRDs](https://raw.githubusercontent.com/kubernetes/autoscaler/master/vertical-pod-autoscaler/deploy/vpa-v1-crd-gen.yaml)
    or set `vpa.enabled=false` to disable VPA.

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
     # or if the chain of credential providers should be used:
     #AWS_USE_CREDENTIALS_CHAIN: dHJ1ZQ==
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
see the [examples](examples/) directory and [translation of `DNSEntries` examples](docs/usage/dnsentry_translation.md)

### Automatic creation of DNS entries for services and ingresses

Using the source controllers, it is also possible to create DNS entries for services (of type `LoadBalancer`)
and ingresses automatically. The resources only need to be annotated with some special values.
In this case ensure that the source controllers are enabled on startup of the DNS controller manager, i.e. the
value of the command line option `--controllers` must contain `dnscontrollers` or equal to `all`.
The DNS source controllers watch resources on the default cluster and create DNS entries on
the target cluster. As there can be multiple controllers active on the same cluster, you may
need to set the correct `DNSClass` both for the controller and for the source resource by
setting the annotation `dns.gardener.cloud/class`. The default value for the `DNSClass` is `gardendns`.

**Note**: If you delegate the DNS management for shoot resources to Gardener via the 
[shoot-dns-service extension](https://github.com/gardener/gardener-extension-shoot-dns-service),
the correct annotation is `dns.gardener.cloud/class=garden`.

Here is an example for annotating a service (same as `examples/50-service-with-dns.yaml`):

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: echo.my-dns-domain.com
    dns.gardener.cloud/ttl: "500"
    # If you are delegating the DNS Management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/dns_names/)
    #dns.gardener.cloud/class: garden
    # To temporarily skip reconciliation of created entries
    #dns.gardener.cloud/ignore: "true"
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

#### `A` DNS records with alias targets for provider type AWS-Route53 and AWS load balancers

For AWS-Route53 and AWS load balancers, `A` DNS records with alias target are created instead of `CNAME` 
as an optimisation.

To support dual-stack IP addresses in this case, set one of these annotations:

-  `service.beta.kubernetes.io/aws-load-balancer-ip-address-type=dualstack` (services only)
- `dns.gardener.cloud/ip-stack=dual-stack` (ingresses, services or dnsentries)

In this case, both `A` and `AAAA` records with alias target records are created.

With annotation `dns.gardener.cloud/ip-stack=ipv6`, only an `AAAA` record with alias target is created.

### Automatic creation of DNS entries for gateways

There are source controllers for `Gateways` from [Istio](https://github.com/istio/istio) or the new Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/).
By annotating the `Gateway` resource with the `dns.gardener.cloud/dnsnames` annotation, DNS entries are managed automatically for the hosts.

#### Istio gateways

For Istio, gateways for API versions `networking.istio.io/v1`, `networking.istio.io/v1beta1`, and `networking.istio.io/v1alpha3` are supported.

To enable automatic management of `DNSEntries`, annotate the Istio `Gateway` resource with `dns.gardener.cloud/dnsnames="*"`.
The domain names are extracted from the `spec.servers.hosts` field and from the field `spec.hosts` of related `VirtualService` resources.  

The determination of the `DNSEntry` targets, typically the IP addresses or hostnames of the load balancer, follows these steps:
1. If the `dns.gardener.cloud/targets` annotation is provided, its value is used.
   This value is expected to be a comma-separated list of the load balancer's IP addresses or hostnames.
2. Alternatively, if the `dns.gardener.cloud/ingress` annotation is set, the IP addresses or hostnames are derived from the status 
   of the `Ingress` resource. This resource is identified by its name, which can be in the format `<namespace>/<name>` or simply `<name>`.
   In the latter case, the Gateway resource's namespace is assumed.
3. If neither of these annotations is provided, it is assumed that the Gateway `spec.selector` field in Istio matches
   a `Service` resource of type `LoadBalancer`. In this case, the targets are obtained from the service load balancer's status.

```yaml
apiVersion: networking.istio.io/v1
kind: Gateway
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    #dns.gardener.cloud/ttl: "500"
    # If you are delegating the DNS Management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/dns_names/)
    #dns.gardener.cloud/class: garden
    # To temporarily skip reconciliation of created entries
    #dns.gardener.cloud/ignore: "true"
  name: my-gateway
  namespace: default
spec:
  selector:
    istio: ingressgateway
  servers:
    - hosts:
        - uk.example.com
        - eu.example.com
      port:
        name: http
        number: 80
        protocol: HTTP
      tls:
        httpsRedirect: true
    - hosts:
        - uk.example.com
        - eu.example.com
      port:
        name: https-443
        number: 443
        protocol: HTTPS
      tls:
        mode: SIMPLE
        privateKey: /etc/certs/privatekey.pem
        serverCertificate: /etc/certs/servercert.pem
    - hosts:
        - bookinfo-namespace/*.example.com
      port:
        name: https-9443
        number: 9443
        protocol: HTTPS
      tls:
        credentialName: my-secret
        mode: SIMPLE
```

In this case, three `DNSEntries` would be created with domain names `uk.example.com`,  `eu.example.com`,  and `*.example.com`.
As neither `dns.gardener.cloud/targets` or `dns.gardener.cloud/ingress` annotation is provided, the targets need to 
come from the load balancer status of a `Service` resource with the label selector `istio=ingressgateway`.

*Note: Alternatively in this concrete example, you could annotate the `Service` resource with `dns.gardener.cloud/dnsnames="*.example.com"`,
if the domain names are static.* 

See the [Istio tutorial](docs/usage/tutorials/istio-gateways.md) for a more detailed example.

#### Gateway API gateways

The Gateway API versions `gateway.networking.k8s.io/v1` and `gateway.networking.k8s.io/v1beta1` are supported.

To enable automatic management of `DNSEntries`, annotate the Gateway API `Gateway` resource with `dns.gardener.cloud/dnsnames="*"`.
The domain names are extracted from the `spec.listeners.hostnames` field and from the field `spec.hostnames` of related `HTTPRoute` resources.

The targets of the `DNSEntry` are extracted from the `status.addresses` field.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    #dns.gardener.cloud/ttl: "500"
    # If you are delegating the DNS Management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/dns_names/)
    #dns.gardener.cloud/class: garden
    # To temporarily skip reconciliation of created entries
    #dns.gardener.cloud/ignore: "true"
  name: my-gateway
  namespace: default
spec:
  gatewayClassName: my-gateway-class
  listeners:
    - allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"
      hostname: foo.example.com
      name: https
      port: 443
      protocol: HTTPS
      tls: ...
status:
  addresses:
    - type: IPAddress
      value: 1.2.3.4  
```

In this case, a single `DNSEntry` with domain name `foo.example.com` and target IP `1.2.3.4` would be created. 

See the [Gateway API tutorial](docs/usage/tutorials/gateway-api-gateways.md) for a more detailed example.

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

Starting with former release `v0.23`, owner identifier are no longer supported.
Formerly, every DNS Provisioning Controller was responsible for a set of _Owner Identifiers_.
For every DNS record, there was an additional `TXT` DNS record ("metadata record") referencing the owner identifier.
It was decided to remove this feature, as it doubles the number of DNS records without adding
enough value.

With release `v0.24`, the `DNSOwner` resources have been removed completely.

### DNS Classes

Multiple sets of controllers of the DNS ecosystem can run in parallel in
a Kubernetes cluster working on different object set. They are separated by
using different _DNS Classes_. Adding a DNS class annotation to an object of the
DNS ecosystems assigns this object to such a dedicated set of DNS controllers.
This way it is possible to maintain clearly separated set of DNS objects in a
single Kubernetes cluster.

### DNSAnnotation objects

DNS source controllers support the creation of DNS entries for potentially
any kind of resource originally not equipped to describe the generation of
DNS entries. This is done by additionally annotations. Nevertheless, it
might be the case, that those objects are again the result of a generation
process, ether by predefined helm starts or by other higher level controllers.
It is not necessarily possible to influence those generation steps to
additionally generate the desired DNS annotations. 

The typical mechanism in Kubernetes to handle this is to provide mutating
webhooks that enrich the generated objects accordingly. But this mechanism
is basically not intended to support dedicated settings for dedicated instances.
At least it is very strenuous to provide web hooks for every such use case.

Therefore, the DNS ecosystem provided by this project supports an additional
extension mechanism to annotate any kind of object with additional annotations
by supported a dedicated resource, the `DNSAnnotation`. 

The handling of this resource is done by a dedicated controller, the `annotation`
controller. It caches the annotation settings declared by those objects and
makes them accessible for the DNS source controllers.

The DNS source controller responsible for a dedicated kind of resource
,for example Service, reads the object, analyses the annotations, and then decides
what to do with it. Most of the flow is handled by a central library, only
some dedicated resource dependent steps are implemented separately by a
dedicated source controller. The `DNSAnnotation` resource slightly extends this
flow: After reading the object the library additionally checks for the existence
of a `DNSAnnotation` setting for this object by querying the `annotation`
controller's cache. If found, it adds annotations declared there to the original
object prior to the next processing steps.
This way, for example whenever a `Service` without
any DNS related annotation is handled by the controller, and it finds a matching
`DNSAnnotation` setting, the set of actual annotations is enriched accordingly
before the actual processing of the service object is done by the controller.

This `DNSAnnotation` object can be created before or even after the object to
be annotated and will implicitly cause a reprocessing of the original object by
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
- `dnssources`: all DNS Source Controllers. It includes the controllers
  - `ingress-dns`: handle DNS annotations for the standard Kubernetes ingress resource
  - `service-dns`: handle DNS annotations for the standard Kubernetes service resource

- `dnscontrollers`: all DNS Provisioning Controllers. It includes the controllers
  - `compound`: common DNS provisioning controller

- `all`: (default) all controllers

It is also possible to list dedicated controllers by their name.

To restrict the compound DNS provisioning controller to specific provider types,
use the `--provider-types` option.

The following provider types can be selected (comma separated):
- `alicloud-dns`: Alicloud DNS provider
- `aws-route53`: AWS Route 53 provider
- `azure-dns`: Azure DNS provider
- `google-clouddns`: Google CloudDNS provider
- `openstack-designate`: Openstack Designate provider
- `cloudflare-dns`: Cloudflare DNS provider
- `infoblox-dns`: Infoblox DNS provider
- `netlify-dns`: Netlify DNS provider
- `remote`: Remote DNS provider (a dns-controller-manager with enabled remote access service)
- `powerdns`: PowerDNS provider

Here is the complete list of options provided:

```txt
Usage:
  dns-controller-manager [flags]

Flags:
      --accepted-maintainers string                                     accepted maintainer key(s) for crds
      --advanced.batch-size int                                         batch size for change requests (currently only used for aws-route53)
      --advanced.max-retries int                                        maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --alicloud-dns.advanced.batch-size int                            batch size for change requests (currently only used for aws-route53)
      --alicloud-dns.advanced.max-retries int                           maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --alicloud-dns.blocked-zone zone-id                               Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --alicloud-dns.ratelimiter.burst int                              number of burst requests for rate limiter
      --alicloud-dns.ratelimiter.enabled                                enables rate limiter for DNS provider requests
      --alicloud-dns.ratelimiter.qps int                                maximum requests/queries per second
      --annotation.default.pool.size int                                Worker pool size for pool default of controller annotation
      --annotation.pool.size int                                        Worker pool size of controller annotation
      --annotation.setup int                                            number of processors for controller setup of controller annotation
      --aws-route53.advanced.batch-size int                             batch size for change requests (currently only used for aws-route53)
      --aws-route53.advanced.max-retries int                            maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --aws-route53.blocked-zone zone-id                                Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --aws-route53.ratelimiter.burst int                               number of burst requests for rate limiter
      --aws-route53.ratelimiter.enabled                                 enables rate limiter for DNS provider requests
      --aws-route53.ratelimiter.qps int                                 maximum requests/queries per second
      --azure-dns.advanced.batch-size int                               batch size for change requests (currently only used for aws-route53)
      --azure-dns.advanced.max-retries int                              maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --azure-dns.blocked-zone zone-id                                  Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --azure-dns.ratelimiter.burst int                                 number of burst requests for rate limiter
      --azure-dns.ratelimiter.enabled                                   enables rate limiter for DNS provider requests
      --azure-dns.ratelimiter.qps int                                   maximum requests/queries per second
      --azure-private-dns.advanced.batch-size int                       batch size for change requests (currently only used for aws-route53)
      --azure-private-dns.advanced.max-retries int                      maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --azure-private-dns.blocked-zone zone-id                          Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --azure-private-dns.ratelimiter.burst int                         number of burst requests for rate limiter
      --azure-private-dns.ratelimiter.enabled                           enables rate limiter for DNS provider requests
      --azure-private-dns.ratelimiter.qps int                           maximum requests/queries per second
      --bind-address-http string                                        HTTP server bind address
      --blocked-zone zone-id                                            Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --cache-ttl int                                                   Time-to-live for provider hosted zone cache
      --cloudflare-dns.advanced.batch-size int                          batch size for change requests (currently only used for aws-route53)
      --cloudflare-dns.advanced.max-retries int                         maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --cloudflare-dns.blocked-zone zone-id                             Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --cloudflare-dns.ratelimiter.burst int                            number of burst requests for rate limiter
      --cloudflare-dns.ratelimiter.enabled                              enables rate limiter for DNS provider requests
      --cloudflare-dns.ratelimiter.qps int                              maximum requests/queries per second
      --compound.advanced.batch-size int                                batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.advanced.max-retries int                               maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.alicloud-dns.advanced.batch-size int                   batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.alicloud-dns.advanced.max-retries int                  maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.alicloud-dns.blocked-zone zone-id                      Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.alicloud-dns.ratelimiter.burst int                     number of burst requests for rate limiter of controller compound
      --compound.alicloud-dns.ratelimiter.enabled                       enables rate limiter for DNS provider requests of controller compound
      --compound.alicloud-dns.ratelimiter.qps int                       maximum requests/queries per second of controller compound
      --compound.aws-route53.advanced.batch-size int                    batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.aws-route53.advanced.max-retries int                   maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.aws-route53.blocked-zone zone-id                       Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.aws-route53.ratelimiter.burst int                      number of burst requests for rate limiter of controller compound
      --compound.aws-route53.ratelimiter.enabled                        enables rate limiter for DNS provider requests of controller compound
      --compound.aws-route53.ratelimiter.qps int                        maximum requests/queries per second of controller compound
      --compound.azure-dns.advanced.batch-size int                      batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.azure-dns.advanced.max-retries int                     maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.azure-dns.blocked-zone zone-id                         Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.azure-dns.ratelimiter.burst int                        number of burst requests for rate limiter of controller compound
      --compound.azure-dns.ratelimiter.enabled                          enables rate limiter for DNS provider requests of controller compound
      --compound.azure-dns.ratelimiter.qps int                          maximum requests/queries per second of controller compound
      --compound.azure-private-dns.advanced.batch-size int              batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.azure-private-dns.advanced.max-retries int             maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.azure-private-dns.blocked-zone zone-id                 Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.azure-private-dns.ratelimiter.burst int                number of burst requests for rate limiter of controller compound
      --compound.azure-private-dns.ratelimiter.enabled                  enables rate limiter for DNS provider requests of controller compound
      --compound.azure-private-dns.ratelimiter.qps int                  maximum requests/queries per second of controller compound
      --compound.blocked-zone zone-id                                   Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.cache-ttl int                                          Time-to-live for provider hosted zone cache of controller compound
      --compound.cloudflare-dns.advanced.batch-size int                 batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.cloudflare-dns.advanced.max-retries int                maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.cloudflare-dns.blocked-zone zone-id                    Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.cloudflare-dns.ratelimiter.burst int                   number of burst requests for rate limiter of controller compound
      --compound.cloudflare-dns.ratelimiter.enabled                     enables rate limiter for DNS provider requests of controller compound
      --compound.cloudflare-dns.ratelimiter.qps int                     maximum requests/queries per second of controller compound
      --compound.default.pool.size int                                  Worker pool size for pool default of controller compound
      --compound.disable-dnsname-validation                             disable validation of domain names according to RFC 1123. of controller compound
      --compound.disable-zone-state-caching                             disable use of cached dns zone state on changes of controller compound
      --compound.dns-class string                                       Class identifier used to differentiate responsible controllers for entry resources of controller compound
      --compound.dns-delay duration                                     delay between two dns reconciliations of controller compound
      --compound.dns.pool.resync-period duration                        Period for resynchronization for pool dns of controller compound
      --compound.dns.pool.size int                                      Worker pool size for pool dns of controller compound
      --compound.dry-run                                                just check, don't modify of controller compound
      --compound.google-clouddns.advanced.batch-size int                batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.google-clouddns.advanced.max-retries int               maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.google-clouddns.blocked-zone zone-id                   Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.google-clouddns.ratelimiter.burst int                  number of burst requests for rate limiter of controller compound
      --compound.google-clouddns.ratelimiter.enabled                    enables rate limiter for DNS provider requests of controller compound
      --compound.google-clouddns.ratelimiter.qps int                    maximum requests/queries per second of controller compound
      --compound.infoblox-dns.advanced.batch-size int                   batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.infoblox-dns.advanced.max-retries int                  maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.infoblox-dns.blocked-zone zone-id                      Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.infoblox-dns.ratelimiter.burst int                     number of burst requests for rate limiter of controller compound
      --compound.infoblox-dns.ratelimiter.enabled                       enables rate limiter for DNS provider requests of controller compound
      --compound.infoblox-dns.ratelimiter.qps int                       maximum requests/queries per second of controller compound
      --compound.lock-status-check-period duration                      interval for dns lock status checks of controller compound
      --compound.max-metadata-record-deletions-per-reconciliation int   maximum number of metadata owner records that can be deleted per zone reconciliation of controller compound
      --compound.netlify-dns.advanced.batch-size int                    batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.netlify-dns.advanced.max-retries int                   maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.netlify-dns.blocked-zone zone-id                       Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.netlify-dns.ratelimiter.burst int                      number of burst requests for rate limiter of controller compound
      --compound.netlify-dns.ratelimiter.enabled                        enables rate limiter for DNS provider requests of controller compound
      --compound.netlify-dns.ratelimiter.qps int                        maximum requests/queries per second of controller compound
      --compound.openstack-designate.advanced.batch-size int            batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.openstack-designate.advanced.max-retries int           maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.openstack-designate.blocked-zone zone-id               Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.openstack-designate.ratelimiter.burst int              number of burst requests for rate limiter of controller compound
      --compound.openstack-designate.ratelimiter.enabled                enables rate limiter for DNS provider requests of controller compound
      --compound.openstack-designate.ratelimiter.qps int                maximum requests/queries per second of controller compound
      --compound.pool.resync-period duration                            Period for resynchronization of controller compound
      --compound.pool.size int                                          Worker pool size of controller compound
      --compound.powerdns.advanced.batch-size int                       batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.powerdns.advanced.max-retries int                      maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.powerdns.blocked-zone zone-id                          Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.powerdns.ratelimiter.burst int                         number of burst requests for rate limiter of controller compound
      --compound.powerdns.ratelimiter.enabled                           enables rate limiter for DNS provider requests of controller compound
      --compound.powerdns.ratelimiter.qps int                           maximum requests/queries per second of controller compound
      --compound.provider-types string                                  comma separated list of provider types to enable of controller compound
      --compound.providers.pool.resync-period duration                  Period for resynchronization for pool providers of controller compound
      --compound.providers.pool.size int                                Worker pool size for pool providers of controller compound
      --compound.ratelimiter.burst int                                  number of burst requests for rate limiter of controller compound
      --compound.ratelimiter.enabled                                    enables rate limiter for DNS provider requests of controller compound
      --compound.ratelimiter.qps int                                    maximum requests/queries per second of controller compound
      --compound.remote-access-cacert string                            CA who signed client certs file of controller compound
      --compound.remote-access-client-id string                         identifier used for remote access of controller compound
      --compound.remote-access-port int                                 port of remote access server for remote-enabled providers of controller compound
      --compound.remote-access-server-secret-name string                name of secret containing remote access server's certificate of controller compound
      --compound.remote.advanced.batch-size int                         batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.remote.advanced.max-retries int                        maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.remote.blocked-zone zone-id                            Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.remote.ratelimiter.burst int                           number of burst requests for rate limiter of controller compound
      --compound.remote.ratelimiter.enabled                             enables rate limiter for DNS provider requests of controller compound
      --compound.remote.ratelimiter.qps int                             maximum requests/queries per second of controller compound
      --compound.reschedule-delay duration                              reschedule delay after losing provider of controller compound
      --compound.rfc2136.advanced.batch-size int                        batch size for change requests (currently only used for aws-route53) of controller compound
      --compound.rfc2136.advanced.max-retries int                       maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53) of controller compound
      --compound.rfc2136.blocked-zone zone-id                           Blocks a zone given in the format zone-id from a provider as if the zone is not existing. of controller compound
      --compound.rfc2136.ratelimiter.burst int                          number of burst requests for rate limiter of controller compound
      --compound.rfc2136.ratelimiter.enabled                            enables rate limiter for DNS provider requests of controller compound
      --compound.rfc2136.ratelimiter.qps int                            maximum requests/queries per second of controller compound
      --compound.secrets.pool.size int                                  Worker pool size for pool secrets of controller compound
      --compound.setup int                                              number of processors for controller setup of controller compound
      --compound.ttl int                                                Default time-to-live for DNS entries. Defines how long the record is kept in cache by DNS servers or resolvers. of controller compound
      --compound.zonepolicies.pool.size int                             Worker pool size for pool zonepolicies of controller compound
      --config string                                                   config file
  -c, --controllers string                                              comma separated list of controllers to start (<name>,<group>,all)
      --cpuprofile string                                               set file for cpu profiling
      --default.pool.resync-period duration                             Period for resynchronization for pool default
      --default.pool.size int                                           Worker pool size for pool default
      --disable-dnsname-validation                                      disable validation of domain names according to RFC 1123.
      --disable-namespace-restriction                                   disable access restriction for namespace local access only
      --disable-zone-state-caching                                      disable use of cached dns zone state on changes
      --dns-class string                                                identifier used to differentiate responsible controllers for entries, Class identifier used to differentiate responsible controllers for entry resources, identifier used to differentiate responsible controllers for providers
      --dns-delay duration                                              delay between two dns reconciliations
      --dns-target-class string                                         identifier used to differentiate responsible dns controllers for target entries, identifier used to differentiate responsible dns controllers for target providers
      --dns.pool.resync-period duration                                 Period for resynchronization for pool dns
      --dns.pool.size int                                               Worker pool size for pool dns
      --dnsentry-source.default.pool.resync-period duration             Period for resynchronization for pool default of controller dnsentry-source
      --dnsentry-source.default.pool.size int                           Worker pool size for pool default of controller dnsentry-source
      --dnsentry-source.dns-class string                                identifier used to differentiate responsible controllers for entries of controller dnsentry-source
      --dnsentry-source.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries of controller dnsentry-source
      --dnsentry-source.exclude-domains stringArray                     excluded domains of controller dnsentry-source
      --dnsentry-source.key string                                      selecting key for annotation of controller dnsentry-source
      --dnsentry-source.pool.resync-period duration                     Period for resynchronization of controller dnsentry-source
      --dnsentry-source.pool.size int                                   Worker pool size of controller dnsentry-source
      --dnsentry-source.target-creator-label-name string                label name to store the creator for generated DNS entries of controller dnsentry-source
      --dnsentry-source.target-creator-label-value string               label value for creator label of controller dnsentry-source
      --dnsentry-source.target-name-prefix string                       name prefix in target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-namespace string                         target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-realms string                            realm(s) to use for generated DNS entries of controller dnsentry-source
      --dnsentry-source.targets.pool.size int                           Worker pool size for pool targets of controller dnsentry-source
      --dnsprovider-replication.default.pool.resync-period duration     Period for resynchronization for pool default of controller dnsprovider-replication
      --dnsprovider-replication.default.pool.size int                   Worker pool size for pool default of controller dnsprovider-replication
      --dnsprovider-replication.dns-class string                        identifier used to differentiate responsible controllers for providers of controller dnsprovider-replication
      --dnsprovider-replication.dns-target-class string                 identifier used to differentiate responsible dns controllers for target providers of controller dnsprovider-replication
      --dnsprovider-replication.pool.resync-period duration             Period for resynchronization of controller dnsprovider-replication
      --dnsprovider-replication.pool.size int                           Worker pool size of controller dnsprovider-replication
      --dnsprovider-replication.target-creator-label-name string        label name to store the creator for replicated DNS providers of controller dnsprovider-replication
      --dnsprovider-replication.target-creator-label-value string       label value for creator label of controller dnsprovider-replication
      --dnsprovider-replication.target-name-prefix string               name prefix in target namespace for cross cluster replication of controller dnsprovider-replication
      --dnsprovider-replication.target-namespace string                 target namespace for cross cluster generation of controller dnsprovider-replication
      --dnsprovider-replication.target-realms string                    realm(s) to use for replicated DNS provider of controller dnsprovider-replication
      --dnsprovider-replication.targets.pool.size int                   Worker pool size for pool targets of controller dnsprovider-replication
      --dry-run                                                         just check, don't modify
      --enable-profiling                                                enables profiling server at path /debug/pprof (needs option --server-port-http)
      --exclude-domains stringArray                                     excluded domains
      --force-crd-update                                                enforce update of crds even they are unmanaged
      --google-clouddns.advanced.batch-size int                         batch size for change requests (currently only used for aws-route53)
      --google-clouddns.advanced.max-retries int                        maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --google-clouddns.blocked-zone zone-id                            Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --google-clouddns.ratelimiter.burst int                           number of burst requests for rate limiter
      --google-clouddns.ratelimiter.enabled                             enables rate limiter for DNS provider requests
      --google-clouddns.ratelimiter.qps int                             maximum requests/queries per second
      --grace-period duration                                           inactivity grace period for detecting end of cleanup for shutdown
  -h, --help                                                            help for dns-controller-manager
      --httproutes.pool.size int                                        Worker pool size for pool httproutes
      --infoblox-dns.advanced.batch-size int                            batch size for change requests (currently only used for aws-route53)
      --infoblox-dns.advanced.max-retries int                           maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --infoblox-dns.blocked-zone zone-id                               Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --infoblox-dns.ratelimiter.burst int                              number of burst requests for rate limiter
      --infoblox-dns.ratelimiter.enabled                                enables rate limiter for DNS provider requests
      --infoblox-dns.ratelimiter.qps int                                maximum requests/queries per second
      --ingress-dns.default.pool.resync-period duration                 Period for resynchronization for pool default of controller ingress-dns
      --ingress-dns.default.pool.size int                               Worker pool size for pool default of controller ingress-dns
      --ingress-dns.dns-class string                                    identifier used to differentiate responsible controllers for entries of controller ingress-dns
      --ingress-dns.dns-target-class string                             identifier used to differentiate responsible dns controllers for target entries of controller ingress-dns
      --ingress-dns.exclude-domains stringArray                         excluded domains of controller ingress-dns
      --ingress-dns.key string                                          selecting key for annotation of controller ingress-dns
      --ingress-dns.pool.resync-period duration                         Period for resynchronization of controller ingress-dns
      --ingress-dns.pool.size int                                       Worker pool size of controller ingress-dns
      --ingress-dns.target-creator-label-name string                    label name to store the creator for generated DNS entries of controller ingress-dns
      --ingress-dns.target-creator-label-value string                   label value for creator label of controller ingress-dns
      --ingress-dns.target-name-prefix string                           name prefix in target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-namespace string                             target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-realms string                                realm(s) to use for generated DNS entries of controller ingress-dns
      --ingress-dns.targets.pool.size int                               Worker pool size for pool targets of controller ingress-dns
      --istio-gateways-dns.default.pool.resync-period duration          Period for resynchronization for pool default of controller istio-gateways-dns
      --istio-gateways-dns.default.pool.size int                        Worker pool size for pool default of controller istio-gateways-dns
      --istio-gateways-dns.dns-class string                             identifier used to differentiate responsible controllers for entries of controller istio-gateways-dns
      --istio-gateways-dns.dns-target-class string                      identifier used to differentiate responsible dns controllers for target entries of controller istio-gateways-dns
      --istio-gateways-dns.exclude-domains stringArray                  excluded domains of controller istio-gateways-dns
      --istio-gateways-dns.key string                                   selecting key for annotation of controller istio-gateways-dns
      --istio-gateways-dns.pool.resync-period duration                  Period for resynchronization of controller istio-gateways-dns
      --istio-gateways-dns.pool.size int                                Worker pool size of controller istio-gateways-dns
      --istio-gateways-dns.target-creator-label-name string             label name to store the creator for generated DNS entries of controller istio-gateways-dns
      --istio-gateways-dns.target-creator-label-value string            label value for creator label of controller istio-gateways-dns
      --istio-gateways-dns.target-name-prefix string                    name prefix in target namespace for cross cluster generation of controller istio-gateways-dns
      --istio-gateways-dns.target-namespace string                      target namespace for cross cluster generation of controller istio-gateways-dns
      --istio-gateways-dns.target-realms string                         realm(s) to use for generated DNS entries of controller istio-gateways-dns
      --istio-gateways-dns.targets.pool.size int                        Worker pool size for pool targets of controller istio-gateways-dns
      --istio-gateways-dns.targetsources.pool.size int                  Worker pool size for pool targetsources of controller istio-gateways-dns
      --istio-gateways-dns.virtualservices.pool.size int                Worker pool size for pool virtualservices of controller istio-gateways-dns
      --k8s-gateways-dns.default.pool.resync-period duration            Period for resynchronization for pool default of controller k8s-gateways-dns
      --k8s-gateways-dns.default.pool.size int                          Worker pool size for pool default of controller k8s-gateways-dns
      --k8s-gateways-dns.dns-class string                               identifier used to differentiate responsible controllers for entries of controller k8s-gateways-dns
      --k8s-gateways-dns.dns-target-class string                        identifier used to differentiate responsible dns controllers for target entries of controller k8s-gateways-dns
      --k8s-gateways-dns.exclude-domains stringArray                    excluded domains of controller k8s-gateways-dns
      --k8s-gateways-dns.httproutes.pool.size int                       Worker pool size for pool httproutes of controller k8s-gateways-dns
      --k8s-gateways-dns.key string                                     selecting key for annotation of controller k8s-gateways-dns
      --k8s-gateways-dns.pool.resync-period duration                    Period for resynchronization of controller k8s-gateways-dns
      --k8s-gateways-dns.pool.size int                                  Worker pool size of controller k8s-gateways-dns
      --k8s-gateways-dns.target-creator-label-name string               label name to store the creator for generated DNS entries of controller k8s-gateways-dns
      --k8s-gateways-dns.target-creator-label-value string              label value for creator label of controller k8s-gateways-dns
      --k8s-gateways-dns.target-name-prefix string                      name prefix in target namespace for cross cluster generation of controller k8s-gateways-dns
      --k8s-gateways-dns.target-namespace string                        target namespace for cross cluster generation of controller k8s-gateways-dns
      --k8s-gateways-dns.target-realms string                           realm(s) to use for generated DNS entries of controller k8s-gateways-dns
      --k8s-gateways-dns.targets.pool.size int                          Worker pool size for pool targets of controller k8s-gateways-dns
      --key string                                                      selecting key for annotation
      --kubeconfig string                                               default cluster access
      --kubeconfig.conditional-deploy-crds                              deployment of required crds for cluster default only if there is no managed resource in garden namespace deploying it
      --kubeconfig.disable-deploy-crds                                  disable deployment of required crds for cluster default
      --kubeconfig.id string                                            id for cluster default
      --kubeconfig.migration-ids string                                 migration id for cluster default
      --lease-duration duration                                         lease duration
      --lease-name string                                               name for lease object
      --lease-renew-deadline duration                                   lease renew deadline
      --lease-resource-lock string                                      determines which resource lock to use for leader election, defaults to 'leases'
      --lease-retry-period duration                                     lease retry period
      --lock-status-check-period duration                               interval for dns lock status checks
  -D, --log-level string                                                logrus log level
      --maintainer string                                               maintainer key for crds (default "dns-controller-manager")
      --max-metadata-record-deletions-per-reconciliation int            maximum number of metadata owner records that can be deleted per zone reconciliation
      --name string                                                     name used for controller manager (default "dns-controller-manager")
      --namespace string                                                namespace for lease (default "kube-system")
  -n, --namespace-local-access-only                                     enable access restriction for namespace local access only (deprecated)
      --netlify-dns.advanced.batch-size int                             batch size for change requests (currently only used for aws-route53)
      --netlify-dns.advanced.max-retries int                            maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --netlify-dns.blocked-zone zone-id                                Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --netlify-dns.ratelimiter.burst int                               number of burst requests for rate limiter
      --netlify-dns.ratelimiter.enabled                                 enables rate limiter for DNS provider requests
      --netlify-dns.ratelimiter.qps int                                 maximum requests/queries per second
      --omit-lease                                                      omit lease for development
      --openstack-designate.advanced.batch-size int                     batch size for change requests (currently only used for aws-route53)
      --openstack-designate.advanced.max-retries int                    maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --openstack-designate.blocked-zone zone-id                        Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --openstack-designate.ratelimiter.burst int                       number of burst requests for rate limiter
      --openstack-designate.ratelimiter.enabled                         enables rate limiter for DNS provider requests
      --openstack-designate.ratelimiter.qps int                         maximum requests/queries per second
      --plugin-file string                                              directory containing go plugins
      --pool.resync-period duration                                     Period for resynchronization
      --pool.size int                                                   Worker pool size
      --powerdns.advanced.batch-size int                                batch size for change requests (currently only used for aws-route53)
      --powerdns.advanced.max-retries int                               maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --powerdns.blocked-zone zone-id                                   Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --powerdns.ratelimiter.burst int                                  number of burst requests for rate limiter
      --powerdns.ratelimiter.enabled                                    enables rate limiter for DNS provider requests
      --powerdns.ratelimiter.qps int                                    maximum requests/queries per second
      --provider-types string                                           comma separated list of provider types to enable
      --providers string                                                cluster to look for provider objects
      --providers.conditional-deploy-crds                               deployment of required crds for cluster provider only if there is no managed resource in garden namespace deploying it
      --providers.disable-deploy-crds                                   disable deployment of required crds for cluster provider
      --providers.id string                                             id for cluster provider
      --providers.migration-ids string                                  migration id for cluster provider
      --providers.pool.resync-period duration                           Period for resynchronization for pool providers
      --providers.pool.size int                                         Worker pool size for pool providers
      --ratelimiter.burst int                                           number of burst requests for rate limiter
      --ratelimiter.enabled                                             enables rate limiter for DNS provider requests
      --ratelimiter.qps int                                             maximum requests/queries per second
      --remote-access-cacert string                                     CA who signed client certs file
      --remote-access-client-id string                                  identifier used for remote access
      --remote-access-port int                                          port of remote access server for remote-enabled providers
      --remote-access-server-secret-name string                         name of secret containing remote access server's certificate
      --remote.advanced.batch-size int                                  batch size for change requests (currently only used for aws-route53)
      --remote.advanced.max-retries int                                 maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --remote.blocked-zone zone-id                                     Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --remote.ratelimiter.burst int                                    number of burst requests for rate limiter
      --remote.ratelimiter.enabled                                      enables rate limiter for DNS provider requests
      --remote.ratelimiter.qps int                                      maximum requests/queries per second
      --reschedule-delay duration                                       reschedule delay after losing provider
      --rfc2136.advanced.batch-size int                                 batch size for change requests (currently only used for aws-route53)
      --rfc2136.advanced.max-retries int                                maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53)
      --rfc2136.blocked-zone zone-id                                    Blocks a zone given in the format zone-id from a provider as if the zone is not existing.
      --rfc2136.ratelimiter.burst int                                   number of burst requests for rate limiter
      --rfc2136.ratelimiter.enabled                                     enables rate limiter for DNS provider requests
      --rfc2136.ratelimiter.qps int                                     maximum requests/queries per second
      --secrets.pool.size int                                           Worker pool size for pool secrets
      --server-port-http int                                            HTTP server port (serving /healthz, /metrics, ...)
      --service-dns.default.pool.resync-period duration                 Period for resynchronization for pool default of controller service-dns
      --service-dns.default.pool.size int                               Worker pool size for pool default of controller service-dns
      --service-dns.dns-class string                                    identifier used to differentiate responsible controllers for entries of controller service-dns
      --service-dns.dns-target-class string                             identifier used to differentiate responsible dns controllers for target entries of controller service-dns
      --service-dns.exclude-domains stringArray                         excluded domains of controller service-dns
      --service-dns.key string                                          selecting key for annotation of controller service-dns
      --service-dns.pool.resync-period duration                         Period for resynchronization of controller service-dns
      --service-dns.pool.size int                                       Worker pool size of controller service-dns
      --service-dns.target-creator-label-name string                    label name to store the creator for generated DNS entries of controller service-dns
      --service-dns.target-creator-label-value string                   label value for creator label of controller service-dns
      --service-dns.target-name-prefix string                           name prefix in target namespace for cross cluster generation of controller service-dns
      --service-dns.target-namespace string                             target namespace for cross cluster generation of controller service-dns
      --service-dns.target-realms string                                realm(s) to use for generated DNS entries of controller service-dns
      --service-dns.targets.pool.size int                               Worker pool size for pool targets of controller service-dns
      --setup int                                                       number of processors for controller setup
      --target string                                                   target cluster for dns requests
      --target-creator-label-name string                                label name to store the creator for generated DNS entries, label name to store the creator for replicated DNS providers
      --target-creator-label-value string                               label value for creator label
      --target-name-prefix string                                       name prefix in target namespace for cross cluster generation, name prefix in target namespace for cross cluster replication
      --target-namespace string                                         target namespace for cross cluster generation
      --target-realms string                                            realm(s) to use for generated DNS entries, realm(s) to use for replicated DNS provider
      --target.conditional-deploy-crds                                  deployment of required crds for cluster target only if there is no managed resource in garden namespace deploying it
      --target.disable-deploy-crds                                      disable deployment of required crds for cluster target
      --target.id string                                                id for cluster target
      --target.migration-ids string                                     migration id for cluster target
      --targets.pool.size int                                           Worker pool size for pool targets
      --targetsources.pool.size int                                     Worker pool size for pool targetsources
      --ttl int                                                         Default time-to-live for DNS entries. Defines how long the record is kept in cache by DNS servers or resolvers.
  -v, --version                                                         version for dns-controller-manager
      --virtualservices.pool.size int                                   Worker pool size for pool virtualservices
      --watch-gateways-crds.default.pool.size int                       Worker pool size for pool default of controller watch-gateways-crds
      --watch-gateways-crds.pool.size int                               Worker pool size of controller watch-gateways-crds
      --zonepolicies.pool.size int                                      Worker pool size for pool zonepolicies
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
an [anonymous import](./cmd/compound/main.go) of the controller package in the main package
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
in the previous section (see the [`controller` sub package](pkg/controller/provider/compound/controller/controller.go)).

### Setting Up a Controller Manager

One or multiple controller packages can be bundled into a controller manager,
by implementing a main package like [this](./cmd/compound/main.go):

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
embedded into the compound factory like [this](./cmd/compound/main.go):

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

This is shown in a complete [example](./cmd/compound/main.go) using the dns
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
    _ "github.com/gardener/external-dns-management/pkg/controller/provider/netlify"

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

Those clusters can then be separated by registering their names together with
command line option names. These can be used to specify different `kubeconfig`
files for those clusters.

By default, all logical clusters are mapped to the default physical cluster
specified via `--kubeconfig` or default cluster access.

If multiple physical clusters are defined they can be specified by a
corresponding cluster option defining the `kubeconfig` file used to access
this cluster. If no such option is specified the default is used.

Therefore, even if the configuration is prepared for multiple clusters,
such a controller manager can easily work on a single cluster if no special
options are given on the command line.

## Why not use the community `external-dns` solution?

Some of the reasons are context-specific, i.e. relate to Gardener's highly dynamic requirements.

1. Custom resource for DNS entries

DNS entries are explicitly specified as custom resources. As an important side effect, each DNS entry provides an own status. Simply by querying the Kubernetes API, a client can check if a requested DNS entry has been successfully added to the DNS backend, or if an update has already been deployed, or if not to reason about the cause. It also opens for easy extensibility, as DNS entries can be created directly via the Kubernetes API. And it simplifies Day 2 operations, e.g. automatic cleanup of unused entries if a DNS provider is deleted.

2. Management of multiple DNS providers

The Gardener DNS controller uses a custom resource DNSProvider to dynamically manage the backend DNS services. While with external-dns you have to specify the single provider during startup, in the Gardener DNS controller you can add/update/delete providers during runtime with different credentials and/or backends. This is important for a multi-tenant environment as in Gardener, where users can bring their own accounts.

A DNS provider can also restrict its actions on subset of the DNS domains (includes and excludes) for which the credentials are capable to edit.

3. Multi cluster support

The Gardener DNS controller distinguish three different logical Kubernetes clusters: Source cluster, target cluster and runtime cluster. The source cluster is monitored by the DNS source controllers for annotations on ingress and service resources. These controllers then create DNS entries in the target cluster. DNS entries in the target cluster are then reconciled/synchronized with the corresponding DNS backend service by the provider controller. The runtime cluster is the cluster the DNS controller runs on. For example, this enables needed flexibility in the Gardener deployment. The DNS controller runs on the seed cluster. This is also the target cluster. DNS providers and entries resources are created in the corresponding namespace of the shoot control plane, while the source cluster is the shoot cluster itself.

4. Optimizations for handling hundreds of DNS entries

Some DNS backend services are restricted on the API calls per second (e.g. the AWS Route 53 API). To manage hundreds of DNS entries it is important to minimize the number of API calls. The Gardener DNS controller heavily makes usage of caches and batch processing for this reason.
