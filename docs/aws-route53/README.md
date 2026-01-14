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
  #region: ...
  #sessionToken: ...
``` 

## Using Workload Identity - Trust Based Authentication

In the context of [[GEP-26] Workload Identity - Trust Based Authentication](https://github.com/gardener/gardener/issues/9586),
when the dns-controller-manager is deployed on a Gardener seed, you can also use
workload identity federation to authenticate to AWS without the need to manage long-lived access keys.
In this case, a `Secret` containing the data fields `token` and `config` is expected.
This secret is not created manually. Instead, Gardener will automatically create and update
it from a `WorkloadIdentity` resource in the project namespace.

Please see the documentation of the Gardener extension `shoot-dns-service` for more details.

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

The AWS Route53 provider currently supports these routing policies types:

- `weighted` [Weighted Routing Policy](#weighted-routing-policy) 
- `latency` [Latency Routing Policy](#latency-routing-policy)
- `geolocation` [Geolocation Routing Policy](#geolocation-routing-policy)
- `ip-based` [IP-Based Routing Policy](#ip-based-routing-policy)
- `failover` [Failover Routing Policy](#failover-routing-policy)


### Weighted Routing Policy

This supports the weighted routing policy as described [here](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/routing-policy-weighted.html).

Each weighted record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
Weighted routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

**Routing policy parameters:**

| Name            | Required | Description                                                                                                                                |
|-----------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `weight`        | Yes      | The value must be an integer >= 0.                                                                                                         |
| `healthCheckID` | No       | The ID of the health check as defined in AWS Route53 account. It must already be existing and is not managed by the dns-controller-manager |

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

#### Example for a blue/green Deployment

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
    # Note: Currently only supported for aws-route53, google-clouddns, alicloud-dns (see https://github.com/gardener/external-dns-management/tree/master/docs/aws-route53#weighted-routing-policy)
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

### Latency Routing Policy

This supports the latency routing policy as described [here](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/routing-policy-latency.html).

Each latency record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
Latency routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

**Routing policy parameters:**

| Name            | Required | Description                                                                                                                                |
|-----------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `region`        | Yes      | The value must be a valid AWS region name (like `eu-west-1`, `us-east-2`, etc.)                                                                                                   |
| `healthCheckID` | No       | The ID of the health check as defined in AWS Route53 account. It must already be existing and is not managed by the dns-controller-manager |

Example:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: latency-eu-west-1
  namespace: default
spec:
  dnsName: "my.third-service.example.com"
  ttl: 120
  targets:
    - instance1.third-service.example.com
  routingPolicy:
    type: latency # only supported for AWS Route 53
    setIdentifier: eu
    parameters:
      region: "eu-west-1" # AWS region name
```

Creating this routing policy using annotations please adjust the details according to the examples for the weighted routing policy:
[Annotating Ingress or Service Resources with Routing Policy](#annotating-ingress-or-service-resources-with-routing-policy)

### Geolocation Routing Policy

This supports the geolocation routing policy as described [here](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/routing-policy-geo.html).

Each geolocation record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
Geolocation routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

**Routing policy parameters:**

| Name            | Required | Description                                                                                                                                                                                                                                                                                                      |
|-----------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `location`      | Yes      | The value is either a name as used when creating such a record in the AWS console. Alternatively, you can use value using codes for the continent or country. E.g. `continent=AF` is an alias for the value `Africa`, and `country=FR` is an alias for the value `France`. The country codes are from ISO  3166. |
| `healthCheckID` | No       | The ID of the health check as defined in AWS Route53 account. It must already be existing and is not managed by the dns-controller-manager                                                                                                                                                                       |

<details>
<summary>Click here to see a complete list of possible values for `location` </summary>

| Name                                         | Alternative name          | Continent | Country | Subdivision |
|----------------------------------------------|---------------------------|-----------|---------|-------------|
| Afghanistan                                  | country=AF                |           | AF      |             |
| Africa                                       | continent=AF              | AF        |         |             |
| Alabama                                      | country=US,subdivision=AL |           | US      | AL          |
| Alaska                                       | country=US,subdivision=AK |           | US      | AK          |
| Albania                                      | country=AL                |           | AL      |             |
| Algeria                                      | country=DZ                |           | DZ      |             |
| American Samoa                               | country=AS                |           | AS      |             |
| Andorra                                      | country=AD                |           | AD      |             |
| Angola                                       | country=AO                |           | AO      |             |
| Anguilla                                     | country=AI                |           | AI      |             |
| Antarctica                                   | continent=AN              | AN        |         |             |
| Antigua and Barbuda                          | country=AG                |           | AG      |             |
| Argentina                                    | country=AR                |           | AR      |             |
| Arizona                                      | country=US,subdivision=AZ |           | US      | AZ          |
| Arkansas                                     | country=US,subdivision=AR |           | US      | AR          |
| Armenia                                      | country=AM                |           | AM      |             |
| Aruba                                        | country=AW                |           | AW      |             |
| Asia                                         | continent=AS              | AS        |         |             |
| Australia                                    | country=AU                |           | AU      |             |
| Austria                                      | country=AT                |           | AT      |             |
| Azerbaijan                                   | country=AZ                |           | AZ      |             |
| Bahamas                                      | country=BS                |           | BS      |             |
| Bahrain                                      | country=BH                |           | BH      |             |
| Bangladesh                                   | country=BD                |           | BD      |             |
| Barbados                                     | country=BB                |           | BB      |             |
| Belarus                                      | country=BY                |           | BY      |             |
| Belgium                                      | country=BE                |           | BE      |             |
| Belize                                       | country=BZ                |           | BZ      |             |
| Benin                                        | country=BJ                |           | BJ      |             |
| Bermuda                                      | country=BM                |           | BM      |             |
| Bhutan                                       | country=BT                |           | BT      |             |
| Bolivia                                      | country=BO                |           | BO      |             |
| Bonaire                                      | country=BQ                |           | BQ      |             |
| Bosnia and Herzegovina                       | country=BA                |           | BA      |             |
| Botswana                                     | country=BW                |           | BW      |             |
| Brazil                                       | country=BR                |           | BR      |             |
| British Indian Ocean Territory               | country=IO                |           | IO      |             |
| British Virgin Islands                       | country=VG                |           | VG      |             |
| Brunei                                       | country=BN                |           | BN      |             |
| Bulgaria                                     | country=BG                |           | BG      |             |
| Burkina Faso                                 | country=BF                |           | BF      |             |
| Burundi                                      | country=BI                |           | BI      |             |
| California                                   | country=US,subdivision=CA |           | US      | CA          |
| Cambodia                                     | country=KH                |           | KH      |             |
| Cameroon                                     | country=CM                |           | CM      |             |
| Canada                                       | country=CA                |           | CA      |             |
| Cape Verde                                   | country=CV                |           | CV      |             |
| Cayman Islands                               | country=KY                |           | KY      |             |
| Central African Republic                     | country=CF                |           | CF      |             |
| Chad                                         | country=TD                |           | TD      |             |
| Cherkaska oblast                             | country=UA,subdivision=71 |           | UA      | 71          |
| Chernihivska oblast                          | country=UA,subdivision=74 |           | UA      | 74          |
| Chernivetska oblast                          | country=UA,subdivision=77 |           | UA      | 77          |
| Chile                                        | country=CL                |           | CL      |             |
| China                                        | country=CN                |           | CN      |             |
| Cocos [Keeling] Islands                      | country=CC                |           | CC      |             |
| Colombia                                     | country=CO                |           | CO      |             |
| Colorado                                     | country=US,subdivision=CO |           | US      | CO          |
| Comoros                                      | country=KM                |           | KM      |             |
| Congo                                        | country=CD                |           | CD      |             |
| Connecticut                                  | country=US,subdivision=CT |           | US      | CT          |
| Cook Islands                                 | country=CK                |           | CK      |             |
| Costa Rica                                   | country=CR                |           | CR      |             |
| Crimea                                       | country=UA,subdivision=11 |           | UA      | 11          |
| Croatia                                      | country=HR                |           | HR      |             |
| Cuba                                         | country=CU                |           | CU      |             |
| Curaçao                                      | country=CW                |           | CW      |             |
| Cyprus                                       | country=CY                |           | CY      |             |
| Czech Republic                               | country=CZ                |           | CZ      |             |
| Default                                      | country=*                 |           | *       |             |
| Delaware                                     | country=US,subdivision=DE |           | US      | DE          |
| Denmark                                      | country=DK                |           | DK      |             |
| District of Columbia                         | country=US,subdivision=DC |           | US      | DC          |
| Djibouti                                     | country=DJ                |           | DJ      |             |
| Dnipropetrovska oblast                       | country=UA,subdivision=12 |           | UA      | 12          |
| Dominica                                     | country=DM                |           | DM      |             |
| Dominican Republic                           | country=DO                |           | DO      |             |
| Donetska oblast                              | country=UA,subdivision=14 |           | UA      | 14          |
| East Timor                                   | country=TL                |           | TL      |             |
| Ecuador                                      | country=EC                |           | EC      |             |
| Egypt                                        | country=EG                |           | EG      |             |
| El Salvador                                  | country=SV                |           | SV      |             |
| Equatorial Guinea                            | country=GQ                |           | GQ      |             |
| Eritrea                                      | country=ER                |           | ER      |             |
| Estonia                                      | country=EE                |           | EE      |             |
| Ethiopia                                     | country=ET                |           | ET      |             |
| Europe                                       | continent=EU              | EU        |         |             |
| Falkland Islands                             | country=FK                |           | FK      |             |
| Faroe Islands                                | country=FO                |           | FO      |             |
| Federated States of Micronesia               | country=FM                |           | FM      |             |
| Fiji                                         | country=FJ                |           | FJ      |             |
| Finland                                      | country=FI                |           | FI      |             |
| Florida                                      | country=US,subdivision=FL |           | US      | FL          |
| France                                       | country=FR                |           | FR      |             |
| French Guiana                                | country=GF                |           | GF      |             |
| French Polynesia                             | country=PF                |           | PF      |             |
| French Southern Territories                  | country=TF                |           | TF      |             |
| Gabon                                        | country=GA                |           | GA      |             |
| Gambia                                       | country=GM                |           | GM      |             |
| Georgia                                      | country=US,subdivision=GA |           | US      | GA          |
| Germany                                      | country=DE                |           | DE      |             |
| Ghana                                        | country=GH                |           | GH      |             |
| Gibraltar                                    | country                   |           |         |             |
| Greece                                       | country=GR                |           | GR      |             |
| Greenland                                    | country=GL                |           | GL      |             |
| Grenada                                      | country=GD                |           | GD      |             |
| Guadeloupe                                   | country=GP                |           | GP      |             |
| Guam                                         | country=GU                |           | GU      |             |
| Guatemala                                    | country=GT                |           | GT      |             |
| Guernsey                                     | country=GG                |           | GG      |             |
| Guinea                                       | country=GN                |           | GN      |             |
| Guinea-Bissau                                | country=GW                |           | GW      |             |
| Guyana                                       | country=GY                |           | GY      |             |
| Haiti                                        | country=HT                |           | HT      |             |
| Hashemite Kingdom of Jordan                  | country=JO                |           | JO      |             |
| Hawaii                                       | country=US,subdivision=HI |           | US      | HI          |
| Honduras                                     | country=HN                |           | HN      |             |
| Hong Kong                                    | country=HK                |           | HK      |             |
| Hungary                                      | country=HU                |           | HU      |             |
| Iceland                                      | country=IS                |           | IS      |             |
| Idaho                                        | country=US,subdivision=ID |           | US      | ID          |
| Illinois                                     | country=US,subdivision=IL |           | US      | IL          |
| India                                        | country=IN                |           | IN      |             |
| Indiana                                      | country=US,subdivision=IN |           | US      | IN          |
| Indonesia                                    | country=ID                |           | ID      |             |
| Iowa                                         | country=US,subdivision=IA |           | US      | IA          |
| Iran                                         | country=IR                |           | IR      |             |
| Iraq                                         | country=IQ                |           | IQ      |             |
| Ireland                                      | country=IE                |           | IE      |             |
| Isle of Man                                  | country=IM                |           | IM      |             |
| Israel                                       | country=IL                |           | IL      |             |
| Italy                                        | country=IT                |           | IT      |             |
| Ivano-Frankivska oblast                      | country=UA,subdivision=26 |           | UA      | 26          |
| Ivory Coast                                  | country=CI                |           | CI      |             |
| Jamaica                                      | country=JM                |           | JM      |             |
| Japan                                        | country=JP                |           | JP      |             |
| Jersey                                       | country=JE                |           | JE      |             |
| Kansas                                       | country=US,subdivision=KS |           | US      | KS          |
| Kazakhstan                                   | country=KZ                |           | KZ      |             |
| Kentucky                                     | country=US,subdivision=KY |           | US      | KY          |
| Kenya                                        | country=KE                |           | KE      |             |
| Kharkivska oblast                            | country=UA,subdivision=63 |           | UA      | 63          |
| Khersonska oblast                            | country=UA,subdivision=65 |           | UA      | 65          |
| Khmelnytska oblast                           | country=UA,subdivision=68 |           | UA      | 68          |
| Kiribati                                     | country=KI                |           | KI      |             |
| Kirovohradska oblast                         | country=UA,subdivision=35 |           | UA      | 35          |
| Kosovo                                       | country=XK                |           | XK      |             |
| Kuwait                                       | country=KW                |           | KW      |             |
| Kyiv                                         | country=UA,subdivision=30 |           | UA      | 30          |
| Kyivska oblast                               | country=UA,subdivision=32 |           | UA      | 32          |
| Kyrgyzstan                                   | country=KG                |           | KG      |             |
| Laos                                         | country=LA                |           | LA      |             |
| Latvia                                       | country=LV                |           | LV      |             |
| Lebanon                                      | country=LB                |           | LB      |             |
| Lesotho                                      | country=LS                |           | LS      |             |
| Liberia                                      | country=LR                |           | LR      |             |
| Libya                                        | country=LY                |           | LY      |             |
| Liechtenstein                                | country=LI                |           | LI      |             |
| Lithuania                                    | country=LT                |           | LT      |             |
| Louisiana                                    | country=US,subdivision=LA |           | US      | LA          |
| Luhanska oblast                              | country=UA,subdivision=09 |           | UA      | 09          |
| Luxembourg                                   | country=LU                |           | LU      |             |
| Lvivska oblast                               | country=UA,subdivision=46 |           | UA      | 46          |
| Macao                                        | country=MO                |           | MO      |             |
| Macedonia                                    | country=MK                |           | MK      |             |
| Madagascar                                   | country=MG                |           | MG      |             |
| Maine                                        | country=US,subdivision=ME |           | US      | ME          |
| Malawi                                       | country=MW                |           | MW      |             |
| Malaysia                                     | country=MY                |           | MY      |             |
| Maldives                                     | country=MV                |           | MV      |             |
| Mali                                         | country=ML                |           | ML      |             |
| Malta                                        | country=MT                |           | MT      |             |
| Marshall Islands                             | country=MH                |           | MH      |             |
| Martinique                                   | country=MQ                |           | MQ      |             |
| Maryland                                     | country=US,subdivision=MD |           | US      | MD          |
| Massachusetts                                | country=US,subdivision=MA |           | US      | MA          |
| Mauritania                                   | country=MR                |           | MR      |             |
| Mauritius                                    | country=MU                |           | MU      |             |
| Mayotte                                      | country=YT                |           | YT      |             |
| Mexico                                       | country=MX                |           | MX      |             |
| Michigan                                     | country=US,subdivision=MI |           | US      | MI          |
| Minnesota                                    | country=US,subdivision=MN |           | US      | MN          |
| Mississippi                                  | country=US,subdivision=MS |           | US      | MS          |
| Missouri                                     | country=US,subdivision=MO |           | US      | MO          |
| Monaco                                       | country=MC                |           | MC      |             |
| Mongolia                                     | country=MN                |           | MN      |             |
| Montana                                      | country=US,subdivision=MT |           | US      | MT          |
| Montenegro                                   | country=ME                |           | ME      |             |
| Montserrat                                   | country=MS                |           | MS      |             |
| Morocco                                      | country=MA                |           | MA      |             |
| Mozambique                                   | country=MZ                |           | MZ      |             |
| Myanmar [Burma]                              | country=MM                |           | MM      |             |
| Mykolaivska oblast                           | country=UA,subdivision=48 |           | UA      | 48          |
| Namibia                                      | country=NA                |           | NA      |             |
| Nauru                                        | country=NR                |           | NR      |             |
| Nebraska                                     | country=US,subdivision=NE |           | US      | NE          |
| Nepal                                        | country=NP                |           | NP      |             |
| Netherlands                                  | country=NL                |           | NL      |             |
| Nevada                                       | country=US,subdivision=NV |           | US      | NV          |
| New Caledonia                                | country=NC                |           | NC      |             |
| New Hampshire                                | country=US,subdivision=NH |           | US      | NH          |
| New Jersey                                   | country=US,subdivision=NJ |           | US      | NJ          |
| New Mexico                                   | country=US,subdivision=NM |           | US      | NM          |
| New York                                     | country=US,subdivision=NY |           | US      | NY          |
| New Zealand                                  | country=NZ                |           | NZ      |             |
| Nicaragua                                    | country=NI                |           | NI      |             |
| Niger                                        | country=NE                |           | NE      |             |
| Nigeria                                      | country=NG                |           | NG      |             |
| Niue                                         | country=NU                |           | NU      |             |
| Norfolk Island                               | country=NF                |           | NF      |             |
| North America                                | continent=NA              | NA        |         |             |
| North Carolina                               | country=US,subdivision=NC |           | US      | NC          |
| North Dakota                                 | country=US,subdivision=ND |           | US      | ND          |
| North Korea                                  | country=KP                |           | KP      |             |
| Northern Mariana Islands                     | country=MP                |           | MP      |             |
| Norway                                       | country=NO                |           | NO      |             |
| Oceania                                      | continent=OC              | OC        |         |             |
| Odeska oblast                                | country=UA,subdivision=51 |           | UA      | 51          |
| Ohio                                         | country=US,subdivision=OH |           | US      | OH          |
| Oklahoma                                     | country=US,subdivision=OK |           | US      | OK          |
| Oman                                         | country=OM                |           | OM      |             |
| Oregon                                       | country=US,subdivision=OR |           | US      | OR          |
| Pakistan                                     | country=PK                |           | PK      |             |
| Palau                                        | country=PW                |           | PW      |             |
| Palestine                                    | country=PS                |           | PS      |             |
| Panama                                       | country=PA                |           | PA      |             |
| Papua New Guinea                             | country=PG                |           | PG      |             |
| Paraguay                                     | country=PY                |           | PY      |             |
| Pennsylvania                                 | country=US,subdivision=PA |           | US      | PA          |
| Peru                                         | country=PE                |           | PE      |             |
| Philippines                                  | country=PH                |           | PH      |             |
| Pitcairn Islands                             | country=PN                |           | PN      |             |
| Poland                                       | country=PL                |           | PL      |             |
| Poltavska oblast                             | country=UA,subdivision=53 |           | UA      | 53          |
| Portugal                                     | country=PT                |           | PT      |             |
| Puerto Rico                                  | country=PR                |           | PR      |             |
| Qatar                                        | country=QA                |           | QA      |             |
| Republic of Korea                            | country=KR                |           | KR      |             |
| Republic of Moldova                          | country=MD                |           | MD      |             |
| Republic of the Congo                        | country=CG                |           | CG      |             |
| Rhode Island                                 | country=US,subdivision=RI |           | US      | RI          |
| Rivnenska oblast                             | country=UA,subdivision=56 |           | UA      | 56          |
| Romania                                      | country=RO                |           | RO      |             |
| Russia                                       | country=RU                |           | RU      |             |
| Rwanda                                       | country=RW                |           | RW      |             |
| Réunion                                      | country=RE                |           | RE      |             |
| Saint Barthélemy                             | country=BL                |           | BL      |             |
| Saint Helena                                 | country=SH                |           | SH      |             |
| Saint Kitts and Nevis                        | country=KN                |           | KN      |             |
| Saint Lucia                                  | country=LC                |           | LC      |             |
| Saint Martin                                 | country=MF                |           | MF      |             |
| Saint Pierre and Miquelon                    | country=PM                |           | PM      |             |
| Saint Vincent and the Grenadines             | country=VC                |           | VC      |             |
| Samoa                                        | country=WS                |           | WS      |             |
| San Marino                                   | country=SM                |           | SM      |             |
| Saudi Arabia                                 | country=SA                |           | SA      |             |
| Senegal                                      | country=SN                |           | SN      |             |
| Serbia                                       | country=RS                |           | RS      |             |
| Sevastopol                                   | country=UA,subdivision=20 |           | UA      | 20          |
| Seychelles                                   | country=SC                |           | SC      |             |
| Sierra Leone                                 | country=SL                |           | SL      |             |
| Singapore                                    | country=SG                |           | SG      |             |
| Sint Maarten                                 | country=SX                |           | SX      |             |
| Slovakia                                     | country=SK                |           | SK      |             |
| Slovenia                                     | country=SI                |           | SI      |             |
| Solomon Islands                              | country=SB                |           | SB      |             |
| Somalia                                      | country=SO                |           | SO      |             |
| South Africa                                 | country=ZA                |           | ZA      |             |
| South America                                | continent=SA              | SA        |         |             |
| South Carolina                               | country=US,subdivision=SC |           | US      | SC          |
| South Dakota                                 | country=US,subdivision=SD |           | US      | SD          |
| South Georgia and the South Sandwich Islands | country=GS                |           | GS      |             |
| South Sudan                                  | country=SS                |           | SS      |             |
| Spain                                        | country=ES                |           | ES      |             |
| Sri Lanka                                    | country=LK                |           | LK      |             |
| Sudan                                        | country=SD                |           | SD      |             |
| Sumska oblast                                | country=UA,subdivision=59 |           | UA      | 59          |
| Suriname                                     | country=SR                |           | SR      |             |
| Svalbard and Jan Mayen                       | country=SJ                |           | SJ      |             |
| Swaziland                                    | country=SZ                |           | SZ      |             |
| Sweden                                       | country=SE                |           | SE      |             |
| Switzerland                                  | country=CH                |           | CH      |             |
| Syria                                        | country=SY                |           | SY      |             |
| São Tomé and Príncipe                        | country=ST                |           | ST      |             |
| Taiwan                                       | country=TW                |           | TW      |             |
| Tajikistan                                   | country=TJ                |           | TJ      |             |
| Tanzania                                     | country=TZ                |           | TZ      |             |
| Tennessee                                    | country=US,subdivision=TN |           | US      | TN          |
| Ternopilska oblast                           | country=UA,subdivision=61 |           | UA      | 61          |
| Texas                                        | country=US,subdivision=TX |           | US      | TX          |
| Thailand                                     | country=TH                |           | TH      |             |
| Togo                                         | country=TG                |           | TG      |             |
| Tokelau                                      | country=TK                |           | TK      |             |
| Tonga                                        | country=TO                |           | TO      |             |
| Trinidad and Tobago                          | country=TT                |           | TT      |             |
| Tunisia                                      | country=TN                |           | TN      |             |
| Turkey                                       | country=TR                |           | TR      |             |
| Turkmenistan                                 | country=TM                |           | TM      |             |
| Turks and Caicos Islands                     | country=TC                |           | TC      |             |
| Tuvalu                                       | country=TV                |           | TV      |             |
| U.S. Minor Outlying Islands                  | country=UM                |           | UM      |             |
| U.S. Virgin Islands                          | country=VI                |           | VI      |             |
| Uganda                                       | country=UG                |           | UG      |             |
| Ukraine                                      | country=UA                |           | UA      |             |
| United Arab Emirates                         | country=AE                |           | AE      |             |
| United Kingdom                               | country=GB                |           | GB      |             |
| United States                                | country=US                |           | US      |             |
| Uruguay                                      | country=UY                |           | UY      |             |
| Utah                                         | country=US,subdivision=UT |           | US      | UT          |
| Uzbekistan                                   | country=UZ                |           | UZ      |             |
| Vanuatu                                      | country=VU                |           | VU      |             |
| Vatican City                                 | country=VA                |           | VA      |             |
| Venezuela                                    | country=VE                |           | VE      |             |
| Vermont                                      | country=US,subdivision=VT |           | US      | VT          |
| Vietnam                                      | country=VN                |           | VN      |             |
| Vinnytska oblast                             | country=UA,subdivision=05 |           | UA      | 05          |
| Virginia                                     | country=US,subdivision=VA |           | US      | VA          |
| Volynska oblast                              | country=UA,subdivision=07 |           | UA      | 07          |
| Wallis and Futuna                            | country=WF                |           | WF      |             |
| Washington                                   | country=US,subdivision=WA |           | US      | WA          |
| West Virginia                                | country=US,subdivision=WV |           | US      | WV          |
| Wisconsin                                    | country=US,subdivision=WI |           | US      | WI          |
| Wyoming                                      | country=US,subdivision=WY |           | US      | WY          |
| Yemen                                        | country=YE                |           | YE      |             |
| Zakarpatska oblast                           | country=UA,subdivision=21 |           | UA      | 21          |
| Zambia                                       | country=ZM                |           | ZM      |             |
| Zaporizka oblast                             | country=UA,subdivision=23 |           | UA      | 23          |
| Zhytomyrska oblast                           | country=UA,subdivision=18 |           | UA      | 18          |
| Zimbabwe                                     | country=ZW                |           | ZW      |             |

*Note: Value may be changed. These are the valid set as read from AWS Route 53 in January, 2023.*

</details>

It is recommended to have a `DNSEntry` with the default location named `Default`. 

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: geolocation-default
  namespace: default
spec:
  dnsName: "my.second-service.example.com"
  ttl: 120
  targets:
    - instance1.second-service.example.com
  # routingPolicy is current only supported for AWS Route53 or Google CloudDNS
  routingPolicy:
    type: geolocation # AWS Route 53 specific example
    setIdentifier: default
    parameters:
      location: Default # default location covers geographic locations that you haven't created records for
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: geolocation-europe
  namespace: default
spec:
  dnsName: "my.second-service.example.com"
  ttl: 120
  targets:
    - instance-eu.second-service.example.com
  # routingPolicy is current only supported for AWS Route53 or Google CloudDNS
  routingPolicy:
    type: geolocation # AWS Route 53 specific example
    setIdentifier: eu
    parameters:
      location: "Europe" # either continent, country or subdivision name (only allowed for countries United States or Ukraine), possible names see docs/aws-route53/README.md
      #location: "continent=EU" # alternatively, use continent or country code as described here: https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resource-record-sets-values-geo.html#rrsets-values-geo-location
      #location: "country=FR"
```

Creating this routing policy using annotations please adjust the details according to the examples for the weighted routing policy:
[Annotating Ingress or Service Resources with Routing Policy](#annotating-ingress-or-service-resources-with-routing-policy)

### IP-Based Routing Policy

This supports the IP-based routing policy as described [here](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/routing-policy-ipbased.html).

Each IP-based record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
IP-Based routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

Before the IP-based routing policy can be used, the CIDR collection and blocks have already to been set up in
AWS Route 53. For details please see [Creating a CIDR collection with CIDR locations and blocks](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resource-record-sets-creating-cidr-collection.html).


**Routing policy parameters:**

| Name            | Required | Description                                                                                                                                |
|-----------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `collection`    | Yes      | CIDR collection name. It must already be existing in AWS Route53 account.                                                                  |
| `location`      | Yes      | The location name as specified for the IP address block(s) in the CIDR collection.                                                         |
| `healthCheckID` | No       | The ID of the health check as defined in AWS Route53 account. It must already be existing and is not managed by the dns-controller-manager |

It is recommended to add a `DNSEntry` for the default location (value `*`) to be used for all IP addresses
not matching any defined IP blocks.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: ip-based-default
  namespace: default
spec:
  dnsName: "my.fourth-service.example.com"
  ttl: 120
  targets:
    - instance1.fourth-service.example.com
  routingPolicy:
    type: ip-based # only supported for AWS Route 53
    setIdentifier: default
    parameters:
      collection: "my-collection" # CIDR collection must be already existing
      location: "*" # default
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: ip-based-loc1
  namespace: default
spec:
  dnsName: "my.fourth-service.example.com"
  ttl: 120
  targets:
    - instance2.fourth-service.example.com
  routingPolicy:
    type: ip-based # only supported for AWS Route 53
    setIdentifier: loc1
    parameters:
      collection: "my-collection" # CIDR collection must already be existing
      location: "my-location1" # location name must already be existing
```

Creating this routing policy using annotations please adjust the details according to the examples for the weighted routing policy:
[Annotating Ingress or Service Resources with Routing Policy](#annotating-ingress-or-service-resources-with-routing-policy)

### Failover Routing Policy

This supports the Failover routing policy as described [here](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/routing-policy-failover.html).

Each failover record set is defined by a separate `DNSEntry`. In this way it is possible to use different dns-controller-manager deployments
acting on the same domain names. Every record set needs a `SetIdentifier` which must be unique for all used identifier of the domain name.
IP-Based routing policy is supported for all record types, i.e. `A`, `AAAA`, `CNAME`, and `TXT`.
All entries of the same domain name must have the same record type and TTL.

**Routing policy parameters:**

| Name                          | Required | Description                                                                                                                                |
|-------------------------------|----------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `failoverRecordType`          | Yes      | CIDR collection name. It must already be existing in AWS Route53 account.                                                                  |
| `healthCheckID`               | Yes      | The ID of the health check as defined in AWS Route53 account. It must already be existing and is not managed by the dns-controller-manager |
| `disableEvaluateTargetHealth` | No       | Only applies if target is a AWS ELB. In this case use this parameter to disable the target health check optionally.                        |


Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: aws-failover-primary
  namespace: default
spec:
  dnsName: "my.fiveth-service.example.com"
  ttl: 120
  targets:
    - instance1.fiveth-service.example.com
  routingPolicy:
    type: failover # only supported for AWS Route 53
    setIdentifier: instance1
    parameters:
      failoverRecordType: primary
      healthCheckID: 66666666-1111-4444-aaaa-25810ea11111
      # disableEvaluateTargetHealth: "true" # only used if target is AWS ELB (target health is enabled by default)
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
  # If you are delegating the DNS management to Gardener Shoot DNS Service, uncomment the following line
  #dns.gardener.cloud/class: garden
  name: aws-failover-secondary
  namespace: default
spec:
  dnsName: "my.fiveth-service.example.com"
  ttl: 120
  targets:
    - instance2.fiveth-service.example.com
  routingPolicy:
    type: failover # only supported for AWS Route 53
    setIdentifier: instance2
    parameters:
      failoverRecordType: secondary
      healthCheckID: 66666666-1111-5555-bbbb-25810ea22222
      # disableEvaluateTargetHealth: "true" # only used if target is AWS ELB (target health is enabled by default)
```

Creating this routing policy using annotations please adjust the details according to the examples for the weighted routing policy:
[Annotating Ingress or Service Resources with Routing Policy](#annotating-ingress-or-service-resources-with-routing-policy)
