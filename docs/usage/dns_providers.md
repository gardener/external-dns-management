# DNS Providers

## Introduction

Gardener can manage DNS records on your behalf, so that you can request them via different resource types (see [here](../dns_names)) within the shoot cluster. The domains for which you are permitted to request records, are however restricted and depend on the DNS provider configuration.

## Shoot provider

By default, every shoot cluster is equipped with a default provider. It is the very same provider that manages the shoot cluster's `kube-apiserver` public DNS record (DNS address in your Kubeconfig).

```
kind: Shoot
...
dns:
  domain: shoot.project.default-domain.gardener.cloud
```

You are permitted to request any sub-domain of `.dns.domain` that is not already taken (e.g. `api.shoot.project.default-domain.gardener.cloud`, `*.ingress.shoot.project.default-domain.gardener.cloud`) with this provider.

## Additional providers

If you need to request DNS records for domains not managed by the [default provider](#Shoot-provider), additional providers must be configured in the shoot specification.

For example:
```
kind: Shoot
...
dns:
  domain: shoot.project.default-domain.gardener.cloud
  providers:
  - secretName: my-aws-account
    type: aws-route53
  - secretName: my-gcp-account
    type: google-clouddns
```

> Please consult the [API-Reference](https://gardener.cloud/documentation/references/core/#core.gardener.cloud/v1beta1.DNSProvider) to get a complete list of supported fields and configuration options.

Referenced secrets should exist in the project namespace in the Garden cluster and must comply with the provider specific credentials format. The **External-DNS-Management** project provides corresponding examples ([20-secret-\<provider-name>-credentials.yaml](https://github.com/gardener/external-dns-management/tree/master/examples)) for known providers.
