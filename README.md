# External DNS Management

This project offers an environment to manage external DNS entries for
a kubernetes cluster. It provides a flexible model allowing to
add DNS source objects and DNS provisioning environments by adding
new independent controllers.

## The Model

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
credentials needed to access an external account. These accounts are then
scanned for DNS zones and domain names they support.
This information is then used to dynamically assign `DNSEntry` objects to
dedicated `DNSProvider` objects. If such an assignment can be done by
a provisioning controller then it is _responsible_ for this entry and manages
the corresponding entries in the external environment.

## The Content

This project contains:

- The _API Group_ objects for `DNSEntry` and `DNSProvider`
- A library that can be used to implement _DNS Source Controllers_
- A library that can be used to implement _DNS Provisioning Controllers_
- Source controllers for Services and Ingresses based on annotations.
- Provisioning Controllers for _Amazon Route53_, _Google CloudDNS_, and _Azure DNS_.
- A controller manager hosting all these controllers.

## How to implement Source Controllers

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

## How to implement Provisioning Controllers

Provisioning controllers can be implemented based on the provisioning controller library
in this repository and must implement the
[`provider.DNSHandlerFactory` interface](pkg/dns/provider/interface.go).
This factory returns implementations of the [`provider.DNSHandler` interface](pkg/dns/provider/interface.go)
that does the effective work for a dedicated set of hosted zones.

A provisioning controller can be implemented following this example:

```go
package route53

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

Complete examples are available in the sub packages of `pkg/controller/provider`.
They also show a typical set of implementation structures that help
to structure the implementation of such controllers.

## Setting Up a Controller Manager

One or multiple controller packages can be bundled into a controller manager,
by implementing a main package like [this](cmd/dns/main.go):

```go
package main

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager"

	_ "github.com/<your conroller package>"
	...
)

func main() {
	controllermanager.Start("my-dns-controller-manager", "dns controller manager", "some description")
}
```

## Multiple Cluster Support

The controller implementations provided in this project are prepared to work
with multiple clusters.

Source controllers can read the DNS source objects from one cluster and
manage `DNSEntries` in another one. Therefore they are using two logical
clusters, the default cluster and a `target` cluster.

Provisioning controllers can read the `DNSEntry` objects from one cluster
and read `DNSProvider` objects from another cluster using the logical clusters
`target` and `provider`.

Those clusters can the be separated by registering their names together with
command line option names. These can be used to specify different kubeconfig 
files for those clusters. If a controller manager includes different types
of controllers then corresponding cluster mappings must be provided in
the coding to assign the controller specific logical names to the
registered external ones. For an example see the included
[controller manager](cmd/dns/main.go).
  
By default all logical clusters are mapped to the default physical cluster
specified via `--kubeconfig` or default cluster access.

If multiple physical clusters are defined they can be specified by a
corresponding cluster option defining the kubeconfig file used to access
this cluster. If no such option is specified the default is used.

Therefore, even if the configuration is prepared for multiple clusters,
such a controller manager can easily work on a single cluster if no special
options are given on the command line.
