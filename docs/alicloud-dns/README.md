# Alicloud DNS Provider

This DNS provider allows you to create and manage DNS entries in [Alibaba Cloud DNS](https://www.alibabacloud.com/product/dns). 

## Generate New Access Key

You need to provide an access key (access key ID and secret access key) for Alibaba Cloud to allow the dns-controller-manager to 
authenticate to Alibaba Cloud DNS.

For details see [AccessKey Client](https://github.com/aliyun/alibaba-cloud-sdk-go/blob/master/docs/2-Client-EN.md#accesskey-client).
Currently the `regionId` is fixed to `cn-shanghai`. 

## Using the Access Key

Create a `Secret` resource with the data fields `ACCESS_KEY_ID` and `SECRET_ACCESS_KEY`.
The values are the base64 encoded access key ID and secret access key respectively.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alicloud-credentials
  namespace: default
type: Opaque
data:
  # Replace '...' with values encoded as base64.
  ACCESS_KEY_ID: ...
  SECRET_ACCESS_KEY: ...

  # Alternatively use Gardener cloud provider credentials convention
  #accessKeyID: ...
  #secretAccessKey: ...
``` 

## Routing Policy

The Alibaba Cloud DNS provider supports only the weighted round-robin routing policy:

- `weighted` [Weighted Routing Policy](#weighted-routing-policy)

For more details, please see the Alibaba Cloud DNS documentation at
[Weight-based intelligent DNS resolution](https://www.alibabacloud.com/help/en/dns/intelligent-analysis-of-sub-weight)

### Weighted Routing Policy

Each weighted record set is defined by a separate `DNSEntry`. In this way, it is possible to use different dns-controller-manager deployments acting on the same domain names.

As it is a round-robin routing policy returning only a single IP address per DNS query, it is recommended to use only one target per `DNSEntry` resource.
If multiple targets are specified in the same `DNSEntry`, this is equivalent to creating separate `DNSEntry` resources for each target with the same domain name and weight.

Every record set needs a `SetIdentifier` which must be a string containing only letters, digits, and '-'.

The weighted routing policy is only supported for the `A` and `AAAA` record types.

All entries of the same domain name must have the same record type and TTL. Allowed values for weights are integer values between 1 and 100.

Example:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: alidns-weighted
  namespace: default
spec:
  dnsName: "my.service.example.com"
  ttl: 60
  targets:
    - 1.2.3.4
  routingPolicy:
    type: weighted
    setIdentifier: "route1"
    parameters:
      weight: "10"
```

#### Annotating Ingress or Service Resources with Routing Policy

To specify the routing policy, add an annotation `dns.gardener.cloud/routing-policy`
containing the routing policy section in JSON format to the `Ingress` or `Service` resource.
Example for an `Ingress` resource:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    dns.gardener.cloud/dnsnames: '*'
    # If you are delegating the DNS management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/dns_names/)
    #dns.gardener.cloud/class: garden
    # If you are delegating the certificate management to Gardener, uncomment the following line (see https://gardener.cloud/documentation/guides/administer_shoots/x509_certificates/)
    #cert.gardener.cloud/purpose: managed
    # routing-policy annotation provides the `.spec.routingPolicy` section as JSON
    # Note: Currently only supported for aws-route53, google-clouddns, alicloud-dns
    dns.gardener.cloud/routing-policy: '{"type": "weighted", "setIdentifier": "route1", "parameters": {"weight": "10"}}'
  name: test-ingress-weighted-routing-policy
  namespace: default
spec:
  rules:
    - host: test.ingress.my-dns-domain.com
      http:
        paths:
          - backend:
              service:
                name: my-service
                port:
                  number: 9000
            path: /
            pathType: Prefix
  tls:
    - hosts:
        - test.ingress.my-dns-domain.com
      #secretName: my-cert-secret-name
```