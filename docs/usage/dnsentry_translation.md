# DNSEntry Translation into DNS records

## Creating `A` and/or `AAAA` records

To create an `A` and/or `AAAA` DNS records, the spec of the `DNSEntry` must contains a list of IP addresses as targets.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: myentry-a
  namespace: default
spec:
  dnsName: "myentry-a.my-own-domain.com"
  targets:
  - 1.2.3.4
  - 1.2.3.5
  - 2abc:1234:5678::42
  ttl: 600 # optional to set TTL
```

Two DNS records are created, as can be checked with nslookup:
```bash
$ nslookup -type=A myentry-a.my-own-domain.com
...
Non-authoritative answer:
Name:	myentry-a.my-own-domain.com
Address: 1.2.3.4
Name:	myentry-a.my-own-domain.com
Address: 1.2.3.5

$ nslookup -type=AAAA myentry-a.my-own-domain.com
...
Non-authoritative answer:
myentry-a.dnstest.my-own-domain.com	has AAAA address 2abc:1234:5678::42
```

## Creating a `CNAME` record

To create a `CNAME` DNS record, the target list of the `DNSEntry` must contain exactly one domain name.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: myentry-cname
  namespace: default
spec:
  dnsName: "myentry-cname.my-own-domain.com"
  targets:
  - mytarget.my-own-domain.com
  ttl: 600 # optional to set TTL
```

A `CNAME` DNS record is created as expected:
```bash
$ nslookup -type=CNAME myentry-cname.my-own-domain.com
...
Non-authoritative answer:
myentry-cname.my-own-domain.com	canonical name = mytarget.my-own-domain.com.
```

## Creating `TXT` record

To create a `TXT` DNS record, the texts list of the `DNSEntry` contains the text values.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: myentry-txt
  namespace: default
spec:
  dnsName: "myentry-txt.my-own-domain.com"
  text:
  - "first value"
  - "second value"
  ttl: 600 # optional to set TTL
```

`TXT` DNS records are created as expected:
```bash
$ nslookup -type=TXT myentry-txt.my-own-domain.com
...
Non-authoritative answer:
  myentry-txt.my-own-domain.com	text = "first value"
  myentry-txt.my-own-domain.com	text = "second value"
```

## Creating `A`/`AAAA` records for multiple domain names 

This is a feature to provide a `CNAME` like behaviour for multiple domain names.
The `.spec.targets` list of the `DNSEntry` contains multiple DNS names in this case.
These names are looked up periodically and resolved into their IPv6 an IPv4 addresses.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: myentry-multi-cname
  namespace: default
spec:
  dnsName: "myentry-multi-cname.my-own-domain.com"
  targets:
  - wikipedia.org
  - wikipedia.de
  ttl: 600 # optional to set TTL
  cnameLookupInterval: 200 # optional to specify lookup internal to resolve the DNS names
```

As `wikipedia.org` resolves to `185.15.59.224` and `2a02:ec80:300:ed1a::1` and
`wikipedia.de` to `49.13.55.174` and `2a01:4f8:c012:3f0a::1` (at time of running this example),
you will get these `A` and `AAAA` DNS records:
```bash
$ nslookup -type=A myentry-multi-cname.my-own-domain.com
...
Non-authoritative answer:
Name:	myentry-multi-cname.my-own-domain.com
Address: 185.15.59.224
Name:	myentry-multi-cname.my-own-domain.com
Address: 49.13.55.174

$ nslookup -type=AAAA myentry-multi-cname.my-own-domain.com
...
Non-authoritative answer:
myentry-multi-cname.my-own-domain.com	has AAAA address 2a02:ec80:300:ed1a::1
myentry-multi-cname.my-own-domain.com	has AAAA address 2a01:4f8:c012:3f0a::1
```

> [!NOTE] 
> Using this feature creates reoccuring work load on the dns-controller-manager as the target domain names
> need to be looked up periodically. If the target addressed have changed, the addresses in the created `A`/`AAAA` records
> will be updated automatically. As two more steps are involved, some additional time is needed until your DNS clients will 
> see such changes in comparison with setting the addresses directly as targets.
> The scheduled DNS lookups happen roughly at the set intervals, but timing depends on cluster load and upstream DNS responsiveness.
> Also be aware that this feature can only be used for domain names visible to the dns-controller-manager.

## Creating `A`/`AAAA` records for single domain name

This is a special feature to avoid a `CNAME` record and resolving the domain name into addresses.
The `.spec.targets` list of the `DNSEntry` contains a single DNS name in this case.
These name is looked up periodically and resolved into its current IPv6 an IPv4 addresses.

Example:
```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: myentry-no-cname
  namespace: default
spec:
  dnsName: "myentry-no-cname.my-own-domain.com"
  targets:
  - wikipedia.org
  resolveTargetsToAddresses: true # this field is needed to distinguish from CNAME record creation
  ttl: 600 # optional to set TTL
  cnameLookupInterval: 200 # optional to specify lookup internal to resolve the DNS names
```

As `wikipedia.org` resolves to `185.15.59.224` and `2a02:ec80:300:ed1a::1` (at time of running this example),
you will get these `A` and `AAAA` DNS records:
```bash
$  nslookup -type=A myentry-no-cname.my-own-domain.com
...
Non-authoritative answer:
Name:	myentry-no-cname.my-own-domain.com
Address: 185.15.59.224

$ nslookup -type=AAAA myentry-no-cname.my-own-domain.com
...
Non-authoritative answer:
myentry-no-cname.my-own-domain.com	has AAAA address 2a02:ec80:300:ed1a::1
```

> [!NOTE]
> Using this feature creates reoccuring work load on the dns-controller-manager as the target domain names
> need to be looked up periodically. If the target addressed have changed, the addresses in the created `A`/`AAAA` records
> will be updated automatically. As two more steps are involved, some additional time is needed until your DNS clients will
> see such changes in comparison with setting the addresses directly as targets.
> The scheduled DNS lookups happen roughly at the set intervals, but timing depends on cluster load and upstream DNS responsiveness.
> Also be aware that this feature can only be used for domain names visible to the dns-controller-manager.
