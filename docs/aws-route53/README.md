# AWS Route 53 DNS Provider

This DNS provider allows you to create and manage DNS entries in AWS Route 53. 

## Generate New Access Key

You need to provide an access key (access key ID and secret access key) for AWS to allow the dns-controller-manager to 
authenticate to AWS Route 53.

For details see https://docs.aws.amazon.com/general/latest/gr/managing-aws-access-keys.html

## Required permissions

The user needs permissions on the hosted zone to list and change DNS records. For details on creating an access policy for a user see https://docs.aws.amazon.com/IAM/latest/UserGuide/access_policies_create.html

In this example, the placeholder for the hosted zone is `Z2XXXXXXXXXXXX`

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "VisualEditor0",
            "Effect": "Allow",
            "Action": "route53:ListResourceRecordSets",
            "Resource": "arn:aws:route53:::hostedzone/*"
        },
        {
            "Sid": "VisualEditor1",
            "Effect": "Allow",
            "Action": "route53:GetHostedZone",
            "Resource": "arn:aws:route53:::hostedzone/Z2XXXXXXXXXXXX"
        },
        {
            "Sid": "VisualEditor2",
            "Effect": "Allow",
            "Action": "route53:ListHostedZones",
            "Resource": "*"
        },
        {
            "Sid": "VisualEditor3",
            "Effect": "Allow",
            "Action": "route53:ChangeResourceRecordSets",
            "Resource": "arn:aws:route53:::hostedzone/Z2XXXXXXXXXXXX"
        }
    ]
}
```

## Using the Access Key

Create a `Secret` resource with the data fields `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
The values are the base64 encoded access key ID and secret access key respectively.

```yaml
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
  # optionally specify the region
  #AWS_REGION: ...
  # optionally specify the token
  #AWS_SESSION_TOKEN: ...
  
  # Alternatively use Gardener cloud provider credentials convention
  #accessKeyID: ...
  #secretAccessKey: ...
``` 

## Using the chain of credential providers

Alternatively the credentials can be provided externally, i.e. by using the
chain of credential providers to search for credentials in environment
variables, shared credential file, and EC2 Instance Roles.

In this case create a `Secret` with the data field `AWS_USE_CREDENTIALS_CHAIN` and set the value to 
`true` (encoded as base64). Typical examples are usage of an AWS Web Identity provider or
[IAM role assigned to the service account](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html).

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aws-credentials
  namespace: default
type: Opaque
data:
  AWS_USE_CREDENTIALS_CHAIN: dHJ1ZQ==
  # optionally specify the region
  #AWS_REGION: ...
```

You may need to mount an additional volume as the AWS client expects environment variable with token path and volume mount with the token file.
See Helm chart values `custom.volumes` and `custom.volumeMounts`.

## Routing Policy

The AWS Route53 provider supports currently only the `weighted` routing policy.

### Weighted Routing Policy

Each weighted record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
Weighted routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

#### Annotating Ingress or Service Resources with Routing Policy

To specify the routing policy, add an annotation `dns.gardener.cloud/routing-policy`
containing the routing policy section in JSON format to the `Ingress` or `Service` resource.
E.g. for an ingress resource:

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
    # Note: Currently only supported for aws-route53 (see https://github.com/gardener/external-dns-management/tree/master/docs/aws-route53#weighted-routing-policy)
    dns.gardener.cloud/routing-policy: '{"type": "weighted", "setIdentifier": "my-id", "parameters": {"weight": "10"}}'
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

#### Example for A/B testing

You want to perform an A/B testing for a service using the domain name `my.service.example.com`.
You want that 90% goes to instance A and 10% to instance B.
You can create these two `DNSEntries` using the same domain name, but different set identifiers

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
    #dns.gardener.cloud/class: garden
  name: instance-a
  namespace: default
spec:
  dnsName: "my.service.example.com"
  ttl: 120
  targets:
    - instance-a.service.example.com
  routingPolicy:
    type: weighted
    setIdentifier: instance-a
    parameters:
      weight: "90"
```

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
    #dns.gardener.cloud/class: garden  
  name: instance-a
  namespace: default
spec:
  dnsName: "my.service.example.com"
  ttl: 120
  targets:
    - instance-b.service.example.com
  routingPolicy:
    type: weighted
    setIdentifier: instance-b
    parameters:
      weight: "10"
```

### Example for a blue/green Deployment

You want to use a blue/green deployment for your service.
Initially you want to activate the `blue` deployment.
Blue and green deployment are located on different clusters, maybe even using different dns-controller-managers (seeds in case of Gardener)

On the blue cluster create a `DNSEntry` with weight 1:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
    #dns.gardener.cloud/class: garden
  name: blue
  namespace: default
spec:
  dnsName: "ha.service.example.com"
  ttl: 60
  targets:
    - 1.2.3.4
  routingPolicy:
    type: weighted
    setIdentifier: blue
    parameters:
      weight: "1"
```

On the green cluster create a `DNSEntry` with weight 0:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
    #dns.gardener.cloud/class: garden
  name: green
  namespace: default
spec:
  dnsName: "ha.service.example.com"
  ttl: 60
  targets:
    - 6.7.8.9
  routingPolicy:
    type: weighted
    setIdentifier: green
    parameters:
      weight: "0"
```

The DNS resolution will return the IP address of the `blue` deployment with this configuration.

To switch the service from `blue` to `green`, first change the weight of the `green` `DNSEntry` to `"1"`.
Wait for DNS propagation according to the TTL (here 60 seconds), then change the weight of the `blue` `DNSEntry` to `"0"`.
After a second wait round for DNS propagation, all DNS resolution should now only return the IP address of the `green`  deployment.