#!/bin/python
# should be started from project base directory

# helper script to regenerate helm chart file: partial of charts/external-dns-management/templates/deployment.yaml


import re
import os

helpFilename = "/tmp/dns-controller-manager-help.txt"
rc = os.system("make build-local && ./dns-controller-manager --help | grep ' --' > {}".format(helpFilename))
if rc != 0:
  exit(rc)
f = open(helpFilename,"r")
options = f.read()
os.remove(helpFilename)

def toCamelCase(name):
  str = ''.join(x.capitalize() for x in re.split("[.-]", name))
  str = str[0].lower() + str[1:]
  str = str.replace("alicloudDns", "alicloudDNS")
  str = str.replace("azureDns", "azureDNS")
  str = str.replace("googleClouddns", "googleCloudDNS")
  str = str.replace("ingressDns", "ingressDNS")
  str = str.replace("serviceDns", "serviceDNS")
  str = str.replace("googleClouddns", "googleCloudDNS")
  str = str.replace("cloudflareDns", "cloudflareDNS")
  str = str.replace("infobloxDns", "infobloxDNS")
  return str

excluded = {"name", "help", "identifier", "dry-run",
  "cache-dir", "alicloud-dns.cache-dir", "aws-route53.cache-dir", "azure-dns.cache-dir", "google-clouddns.cache-dir",
  "openstack-designate.cache-dir", "cloudflare-dns.cache-dir", "infoblox-dns.cache-dir"}
for line in options.split("\n"):
    m = re.match(r"\s+(?:-[^-]+)?--(\S+)\s", line)
    if m:
      name = m.group(1)
      if name != "" and not name in excluded:
        camelCase = toCamelCase(name)
        txt = """        {{- if .Values.configuration.%s }}
        - --%s={{ .Values.configuration.%s }}
        {{- end }}""" % (camelCase, name, camelCase)
        print(txt)

defaultValues = {
  "controllers": "all",
  "persistentCache": "false",
  "persistentCacheStorageSize": "1Gi",
  "persistentCacheStorageSizeAlicloud": "20Gi",
  "serverPortHttp": "8080",
  "ttl": 120,
}

print("configuration:")
for line in options.split("\n"):
    m = re.match(r"\s+(?:-[^-]+)?--(\S+)\s", line)
    if m:
      name = m.group(1)
      if name != "" and not name in excluded:
        camelCase = toCamelCase(name)
        if camelCase in defaultValues:
            txt = "  %s: %s" % (camelCase, defaultValues[camelCase])
        else:
            txt = "# %s:" % camelCase
        print(txt)

