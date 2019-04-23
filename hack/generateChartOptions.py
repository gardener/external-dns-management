#!/bin/python

# helper script to regenerate helm chart file: partial of charts/external-dns-management/templates/deployment.yaml


import re

options = """
alicloud-dns.cache-ttl
alicloud-dns.default.pool.size
alicloud-dns.dns-class
alicloud-dns.dns-delay
alicloud-dns.dns.pool.resync-period
alicloud-dns.dns.pool.size
alicloud-dns.dry-run
alicloud-dns.identifier
alicloud-dns.ownerids.pool.size
alicloud-dns.providers.pool.resync-period
alicloud-dns.providers.pool.size
alicloud-dns.secrets.pool.size
alicloud-dns.setup
alicloud-dns.ttl
aws-route53.cache-ttl
aws-route53.default.pool.size
aws-route53.dns-class
aws-route53.dns-delay
aws-route53.dns.pool.resync-period
aws-route53.dns.pool.size
aws-route53.dry-run
aws-route53.identifier
aws-route53.ownerids.pool.size
aws-route53.providers.pool.resync-period
aws-route53.providers.pool.size
aws-route53.secrets.pool.size
aws-route53.setup
aws-route53.ttl
azure-dns.cache-ttl
azure-dns.default.pool.size
azure-dns.dns-class
azure-dns.dns-delay
azure-dns.dns.pool.resync-period
azure-dns.dns.pool.size
azure-dns.dry-run
azure-dns.identifier
azure-dns.ownerids.pool.size
azure-dns.providers.pool.resync-period
azure-dns.providers.pool.size
azure-dns.secrets.pool.size
azure-dns.setup
azure-dns.ttl
cache-ttl
controllers
cpuprofile
disable-namespace-restriction
dns-class
dns-delay
dns-target-class
dry-run
exclude-domains
google-clouddns.cache-ttl
google-clouddns.default.pool.size
google-clouddns.dns-class
google-clouddns.dns-delay
google-clouddns.dns.pool.resync-period
google-clouddns.dns.pool.size
google-clouddns.dry-run
google-clouddns.identifier
google-clouddns.ownerids.pool.size
google-clouddns.providers.pool.resync-period
google-clouddns.providers.pool.size
google-clouddns.secrets.pool.size
google-clouddns.setup
google-clouddns.ttl
help
identifier
ingress-dns.default.pool.resync-period
ingress-dns.default.pool.size
ingress-dns.dns-class
ingress-dns.dns-target-class
ingress-dns.exclude-domains
ingress-dns.key
ingress-dns.target-name-prefix
ingress-dns.target-namespace
ingress-dns.targets.pool.size
key
kubeconfig
kubeconfig.id
log-level
name
namespace
namespace-local-access-only
omit-lease
openstack-designate.cache-ttl
openstack-designate.default.pool.size
openstack-designate.dns-class
openstack-designate.dns-delay
openstack-designate.dns.pool.resync-period
openstack-designate.dns.pool.size
openstack-designate.dry-run
openstack-designate.identifier
openstack-designate.ownerids.pool.size
openstack-designate.providers.pool.resync-period
openstack-designate.providers.pool.size
openstack-designate.secrets.pool.size
openstack-designate.setup
openstack-designate.ttl
plugin-dir
pool.resync-period
pool.size
server-port-http
service-dns.default.pool.resync-period
service-dns.default.pool.size
service-dns.dns-class
service-dns.dns-target-class
service-dns.exclude-domains
service-dns.key
service-dns.target-name-prefix
service-dns.target-namespace
service-dns.targets.pool.size
setup
target
target-name-prefix
target-namespace
target.id
ttl
"""

def toCamelCase(name):
  str = ''.join(x.capitalize() for x in re.split("[.-]", name))
  str = str[0].lower() + str[1:]
  str = str.replace("alicloudDns", "alicloudDNS")
  str = str.replace("azureDns", "azureDNS")
  str = str.replace("googleClouddns", "googleCloudDNS")
  str = str.replace("ingressDns", "ingressDNS")
  str = str.replace("serviceDns", "serviceDNS")
  str = str.replace("googleClouddns", "googleCloudDNS")
  return str

for name in options.split("\n"):
    if name != "":
        camelCase = toCamelCase(name)
        txt = """        {{- if .Values.configuration.%s }}
        - --%s={{ .Values.configuration.%s }}
        {{- end }}""" % (camelCase, name, camelCase)
        print(txt)
