# GCP Cloud DNS Provider

This DNS provider allows you to create and manage DNS entries in GCP Cloud DNS.

## Generate Service Account

You need to provide a service account and a key (serviceaccount.json) to allow the dns-controller-manager to authenticate and execute calls to Cloud DNS.

For details on Cloud DNS see https://cloud.google.com/dns/docs/zones, and on Service Accounts see https://cloud.google.com/iam/docs/service-accounts

## Required permissions

The service account needs permissions on the hosted zone to list and change DNS records. For details on which permissions or roles are required see https://cloud.google.com/dns/docs/access-control. A possible role is `roles/dns.admin` "DNS Administrator".

Create a key for the configured service account. GCP will generate a `serviceaccount.json` file as key, similar to the example below. Keep this file safe as it won't be accessible again.

```json
{
  "type": "service_account",
  "project_id": "...",
  "private_key_id": "...",
  "private_key": "-----BEGIN PRIVATE KEY----- ... -----END PRIVATE KEY-----\n",
  "client_email": "...",
  "client_id": "...",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/..."
}
```


## Using the Service Account Key

Create a `Secret` resource with the data field `serviceaccount.json` with the value being the base64 encoded string, e.g. with

```bash
$ encoded_key=`cat serviceaccount.json | base64`
$ echo $encoded_key
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: google-credentials
  namespace: default
type: Opaque
data:
  # replace '...' with json key from service account creation (encoded as base64)
  # see https://cloud.google.com/iam/docs/creating-managing-service-accounts
  serviceaccount.json: ...
```

## Using Workload Identity - Trust Based Authentication

In the context of [GEP-26: Workload Identity - Trust Based Authentication](https://github.com/gardener/gardener/issues/9586),
when the dns-controller-manager is deployed on a Gardener seed, you can also use
workload identity federation to authenticate to Google Cloud without managing long-lived service account key.
In this case, a `Secret` containing the data fields `token` and `config` is expected.
This secret is not created manually. Instead, Gardener will automatically create and update
it from a `WorkloadIdentity` resource in the project namespace.

Please see the documentation of the Gardener extension `shoot-dns-service` for more [details](https://github.com/gardener/gardener-extension-shoot-dns-service/blob/master/docs/usage/workloadidentity/gcp.md).

## Routing Policy

The Google CloudDNS provider currently supports these routing policies types:

- `weighted` [Weighted Routing Policy](#weighted-routing-policy)
- `geolocation` [Geolocation Routing Policy](#geolocation-routing-policy)

*Note*: Health checks are not supported.

For more details about these routing policies, please see the Google Cloud DNS documentation at
[Manage DNS routing policies and health checks](https://cloud.google.com/dns/docs/zones/manage-routing-policies)

### Weighted Routing Policy

Each weighted record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be a digit "0", "1", "2", "3", or "4" (representing the index in the 
resource record set policy).
Weighted routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL. Only integral weights >= 0 are allowed.

Example:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: google-weighted
  namespace: default
spec:
  dnsName: "my.service.example.com"
  ttl: 60
  targets:
    - 1.2.3.4
  routingPolicy:
    type: weighted # Google Cloud DNS specific example
    setIdentifier: "0"
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
    dns.gardener.cloud/routing-policy: '{"type": "weighted", "setIdentifier": "0", "parameters": {"weight": "10"}}'
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

### Geolocation Routing Policy

Each geolocation record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must identical to the value of the parameter `location`.
Geolocation routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

At the time of writing (January 2023), Google Cloud only supported Google Cloud regions as the geographic boundaries. Other
geographic boundaries may follow. Please see Google documentation for the current state.

<details>
<summary>Click here to see a list of known possible values</summary>

| Google Cloud region | Physical Location |
|---------------------|-------------------|
| asia-east1 | Changhua County, Taiwan |
| asia-east2 | Hong Kong |
| asia-northeast1 | Tokyo, Japan |
| asia-northeast2 | Osaka, Japan |
| asia-northeast3 | Seoul, South Korea |
| asia-south1 | Mumbai, India |
| asia-south2 | Delhi, India |
| asia-southeast1 | Jurong West, Singapore |
| australia-southeast1 | Sydney, Australia |
| australia-southeast2 | Melbourne, Australia |
| europe-central2 | Warsaw, Poland |
| europe-north2 | Hamina, Finland |
| europe-west1 | St. Ghislain, Belgium |
| europe-west2 | London, England |
| europe-west3 | Frankfurt, Germany |
| europe-west4 | Eemshaven, Netherlands |
| europe-west6 | Zurich, Switzerland |
| europe-west8 | Milan, Italy |
| europe-west9 | Paris, France |
| europe-southwest1 | Madrid, Spain |
| me-west1 | Tel Aviv, Israel, Middle East |
| northamerica-northeast1 | Montréal, Québec |
| northamerica-northeast2 | Toronto, Ontario |
| southamerica-east1 | Osasco, São Paulo |
 | southamerica-west1 |	Santiago, Chile, South America |
| us-central1 | Council Bluffs, Iowa |
| us-east1 | Moncks Corner, South Carolina |
| us-east4 | Ashburn, Virginia |
| us-west1 | The Dalles, Orego |
| us-west2 | Los Angeles, California |
| us-west3 | Salt Lake City, Utah |
| us-west4 | Las Vegas, Nevada |



*Note*: No guarantee for completeness
</details>

Example:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
 annotations:
 # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
 #dns.gardener.cloud/class: garden
 name: google-geo-europe-west3
 namespace: default
spec:
 dnsName: "my.second-service.example.com"
 ttl: 60
 targets:
  - 1.2.3.4
 routingPolicy:
  type: geolocation # Google Cloud DNS specific example
  setIdentifier: "europe-west3"
  parameters:
   location: "europe-west3"
```


Creating this routing policy using annotations please adjust the details according to the examples for the weighted routing policy:
[Annotating Ingress or Service Resources with Routing Policy](#annotating-ingress-or-service-resources-with-routing-policy)

