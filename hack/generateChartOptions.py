#!/bin/python

# helper script to regenerate helm chart file: partial of charts/external-dns-management/templates/deployment.yaml


import re

options = """
      --alicloud-dns.cache-dir string                               Directory to store zone caches (for reload after restart)
      --alicloud-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache
      --alicloud-dns.default.pool.size int                          Worker pool size for pool default of controller alicloud-dns (default: 2)
      --alicloud-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes
      --alicloud-dns.dns-class string                               Identifier used to differentiate responsible controllers for entries
      --alicloud-dns.dns-delay duration                             delay between two dns reconciliations
      --alicloud-dns.dns.pool.resync-period duration                Period for resynchronization of pool dns of controller alicloud-dns (default: 15m0s)
      --alicloud-dns.dns.pool.size int                              Worker pool size for pool dns of controller alicloud-dns (default: 1)
      --alicloud-dns.dry-run                                        just check, don't modify
      --alicloud-dns.identifier string                              Identifier used to mark DNS entries
      --alicloud-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller alicloud-dns (default: 1)
      --alicloud-dns.providers.pool.resync-period duration          Period for resynchronization of pool providers of controller alicloud-dns (default: 10m0s)
      --alicloud-dns.providers.pool.size int                        Worker pool size for pool providers of controller alicloud-dns (default: 2)
      --alicloud-dns.reschedule-delay duration                      reschedule delay after losing provider
      --alicloud-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller alicloud-dns (default: 2)
      --alicloud-dns.setup int                                      number of processors for controller setup
      --alicloud-dns.ttl int                                        Default time-to-live for DNS entries
      --aws-route53.cache-dir string                                Directory to store zone caches (for reload after restart)
      --aws-route53.cache-ttl int                                   Time-to-live for provider hosted zone cache
      --aws-route53.default.pool.size int                           Worker pool size for pool default of controller aws-route53 (default: 2)
      --aws-route53.disable-zone-state-caching                      disable use of cached dns zone state on changes
      --aws-route53.dns-class string                                Identifier used to differentiate responsible controllers for entries
      --aws-route53.dns-delay duration                              delay between two dns reconciliations
      --aws-route53.dns.pool.resync-period duration                 Period for resynchronization of pool dns of controller aws-route53 (default: 15m0s)
      --aws-route53.dns.pool.size int                               Worker pool size for pool dns of controller aws-route53 (default: 1)
      --aws-route53.dry-run                                         just check, don't modify
      --aws-route53.identifier string                               Identifier used to mark DNS entries
      --aws-route53.ownerids.pool.size int                          Worker pool size for pool ownerids of controller aws-route53 (default: 1)
      --aws-route53.providers.pool.resync-period duration           Period for resynchronization of pool providers of controller aws-route53 (default: 10m0s)
      --aws-route53.providers.pool.size int                         Worker pool size for pool providers of controller aws-route53 (default: 2)
      --aws-route53.reschedule-delay duration                       reschedule delay after losing provider
      --aws-route53.secrets.pool.size int                           Worker pool size for pool secrets of controller aws-route53 (default: 2)
      --aws-route53.setup int                                       number of processors for controller setup
      --aws-route53.ttl int                                         Default time-to-live for DNS entries
      --azure-dns.cache-dir string                                  Directory to store zone caches (for reload after restart)
      --azure-dns.cache-ttl int                                     Time-to-live for provider hosted zone cache
      --azure-dns.default.pool.size int                             Worker pool size for pool default of controller azure-dns (default: 2)
      --azure-dns.disable-zone-state-caching                        disable use of cached dns zone state on changes
      --azure-dns.dns-class string                                  Identifier used to differentiate responsible controllers for entries
      --azure-dns.dns-delay duration                                delay between two dns reconciliations
      --azure-dns.dns.pool.resync-period duration                   Period for resynchronization of pool dns of controller azure-dns (default: 15m0s)
      --azure-dns.dns.pool.size int                                 Worker pool size for pool dns of controller azure-dns (default: 1)
      --azure-dns.dry-run                                           just check, don't modify
      --azure-dns.identifier string                                 Identifier used to mark DNS entries
      --azure-dns.ownerids.pool.size int                            Worker pool size for pool ownerids of controller azure-dns (default: 1)
      --azure-dns.providers.pool.resync-period duration             Period for resynchronization of pool providers of controller azure-dns (default: 10m0s)
      --azure-dns.providers.pool.size int                           Worker pool size for pool providers of controller azure-dns (default: 2)
      --azure-dns.reschedule-delay duration                         reschedule delay after losing provider
      --azure-dns.secrets.pool.size int                             Worker pool size for pool secrets of controller azure-dns (default: 2)
      --azure-dns.setup int                                         number of processors for controller setup
      --azure-dns.ttl int                                           Default time-to-live for DNS entries
      --cache-dir string                                            default for all controller "cache-dir" options
      --cache-ttl int                                               default for all controller "cache-ttl" options
      --cloudflare-dns.cache-dir string                             Directory to store zone caches (for reload after restart)
      --cloudflare-dns.cache-ttl int                                Time-to-live for provider hosted zone cache
      --cloudflare-dns.default.pool.size int                        Worker pool size for pool default of controller cloudflare-dns (default: 2)
      --cloudflare-dns.disable-zone-state-caching                   disable use of cached dns zone state on changes
      --cloudflare-dns.dns-class string                             Identifier used to differentiate responsible controllers for entries
      --cloudflare-dns.dns-delay duration                           delay between two dns reconciliations
      --cloudflare-dns.dns.pool.resync-period duration              Period for resynchronization of pool dns of controller cloudflare-dns (default: 15m0s)
      --cloudflare-dns.dns.pool.size int                            Worker pool size for pool dns of controller cloudflare-dns (default: 1)
      --cloudflare-dns.dry-run                                      just check, don't modify
      --cloudflare-dns.identifier string                            Identifier used to mark DNS entries
      --cloudflare-dns.ownerids.pool.size int                       Worker pool size for pool ownerids of controller cloudflare-dns (default: 1)
      --cloudflare-dns.providers.pool.resync-period duration        Period for resynchronization of pool providers of controller cloudflare-dns (default: 10m0s)
      --cloudflare-dns.providers.pool.size int                      Worker pool size for pool providers of controller cloudflare-dns (default: 2)
      --cloudflare-dns.reschedule-delay duration                    reschedule delay after losing provider
      --cloudflare-dns.secrets.pool.size int                        Worker pool size for pool secrets of controller cloudflare-dns (default: 2)
      --cloudflare-dns.setup int                                    number of processors for controller setup
      --cloudflare-dns.ttl int                                      Default time-to-live for DNS entries
  -c, --controllers string                                          comma separated list of controllers to start (<name>,source,target,all) (default "all")
      --cpuprofile string                                           set file for cpu profiling
      --disable-namespace-restriction                               disable access restriction for namespace local access only
      --disable-zone-state-caching                                  default for all controller "disable-zone-state-caching" options
      --dns-class string                                            default for all controller "dns-class" options
      --dns-delay duration                                          default for all controller "dns-delay" options
      --dns-target-class string                                     default for all controller "dns-target-class" options
      --dnsentry-source.default.pool.resync-period duration         Period for resynchronization of pool default of controller dnsentry-source (default: 2m0s)
      --dnsentry-source.default.pool.size int                       Worker pool size for pool default of controller dnsentry-source (default: 2)
      --dnsentry-source.dns-class string                            identifier used to differentiate responsible controllers for entries
      --dnsentry-source.dns-target-class string                     identifier used to differentiate responsible dns controllers for target entries
      --dnsentry-source.exclude-domains stringArray                 excluded domains
      --dnsentry-source.key string                                  selecting key for annotation
      --dnsentry-source.target-creator-label-name string            label name to store the creator for generated DNS entries
      --dnsentry-source.target-creator-label-value string           label value for creator label
      --dnsentry-source.target-name-prefix string                   name prefix in target namespace for cross cluster generation
      --dnsentry-source.target-namespace string                     target namespace for cross cluster generation
      --dnsentry-source.target-owner-id string                      owner id to use for generated DNS entries
      --dnsentry-source.target-realms string                        realm(s) to use for generated DNS entries
      --dnsentry-source.target-set-ignore-owners                    mark generated DNS entries to omit owner based access control
      --dnsentry-source.targets.pool.size int                       Worker pool size for pool targets of controller dnsentry-source (default: 2)
      --dry-run                                                     default for all controller "dry-run" options
      --exclude-domains stringArray                                 default for all controller "exclude-domains" options
      --google-clouddns.cache-dir string                            Directory to store zone caches (for reload after restart)
      --google-clouddns.cache-ttl int                               Time-to-live for provider hosted zone cache
      --google-clouddns.default.pool.size int                       Worker pool size for pool default of controller google-clouddns (default: 2)
      --google-clouddns.disable-zone-state-caching                  disable use of cached dns zone state on changes
      --google-clouddns.dns-class string                            Identifier used to differentiate responsible controllers for entries
      --google-clouddns.dns-delay duration                          delay between two dns reconciliations
      --google-clouddns.dns.pool.resync-period duration             Period for resynchronization of pool dns of controller google-clouddns (default: 15m0s)
      --google-clouddns.dns.pool.size int                           Worker pool size for pool dns of controller google-clouddns (default: 1)
      --google-clouddns.dry-run                                     just check, don't modify
      --google-clouddns.identifier string                           Identifier used to mark DNS entries
      --google-clouddns.ownerids.pool.size int                      Worker pool size for pool ownerids of controller google-clouddns (default: 1)
      --google-clouddns.providers.pool.resync-period duration       Period for resynchronization of pool providers of controller google-clouddns (default: 10m0s)
      --google-clouddns.providers.pool.size int                     Worker pool size for pool providers of controller google-clouddns (default: 2)
      --google-clouddns.reschedule-delay duration                   reschedule delay after losing provider
      --google-clouddns.secrets.pool.size int                       Worker pool size for pool secrets of controller google-clouddns (default: 2)
      --google-clouddns.setup int                                   number of processors for controller setup
      --google-clouddns.ttl int                                     Default time-to-live for DNS entries
      --grace-period duration                                       inactivity grace period for detecting end of cleanup for shutdown
  -h, --help                                                        help for dns-controller-manager
      --identifier string                                           default for all controller "identifier" options
      --ingress-dns.default.pool.resync-period duration             Period for resynchronization of pool default of controller ingress-dns (default: 2m0s)
      --ingress-dns.default.pool.size int                           Worker pool size for pool default of controller ingress-dns (default: 2)
      --ingress-dns.dns-class string                                identifier used to differentiate responsible controllers for entries
      --ingress-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries
      --ingress-dns.exclude-domains stringArray                     excluded domains
      --ingress-dns.key string                                      selecting key for annotation
      --ingress-dns.target-creator-label-name string                label name to store the creator for generated DNS entries
      --ingress-dns.target-creator-label-value string               label value for creator label
      --ingress-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation
      --ingress-dns.target-namespace string                         target namespace for cross cluster generation
      --ingress-dns.target-owner-id string                          owner id to use for generated DNS entries
      --ingress-dns.target-realms string                            realm(s) to use for generated DNS entries
      --ingress-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control
      --ingress-dns.targets.pool.size int                           Worker pool size for pool targets of controller ingress-dns (default: 2)
      --key string                                                  default for all controller "key" options
      --kubeconfig string                                           default cluster access
      --kubeconfig.disable-deploy-crds                              disable deployment of required crds for cluster default
      --kubeconfig.id string                                        id for cluster default
  -D, --log-level string                                            logrus log level
      --name string                                                 name used for controller manager
      --namespace string                                            namespace for lease
  -n, --namespace-local-access-only                                 enable access restriction for namespace local access only (deprecated)
      --omit-lease                                                  omit lease for development
      --openstack-designate.cache-dir string                        Directory to store zone caches (for reload after restart)
      --openstack-designate.cache-ttl int                           Time-to-live for provider hosted zone cache
      --openstack-designate.default.pool.size int                   Worker pool size for pool default of controller openstack-designate (default: 2)
      --openstack-designate.disable-zone-state-caching              disable use of cached dns zone state on changes
      --openstack-designate.dns-class string                        Identifier used to differentiate responsible controllers for entries
      --openstack-designate.dns-delay duration                      delay between two dns reconciliations
      --openstack-designate.dns.pool.resync-period duration         Period for resynchronization of pool dns of controller openstack-designate (default: 15m0s)
      --openstack-designate.dns.pool.size int                       Worker pool size for pool dns of controller openstack-designate (default: 1)
      --openstack-designate.dry-run                                 just check, don't modify
      --openstack-designate.identifier string                       Identifier used to mark DNS entries
      --openstack-designate.ownerids.pool.size int                  Worker pool size for pool ownerids of controller openstack-designate (default: 1)
      --openstack-designate.providers.pool.resync-period duration   Period for resynchronization of pool providers of controller openstack-designate (default: 10m0s)
      --openstack-designate.providers.pool.size int                 Worker pool size for pool providers of controller openstack-designate (default: 2)
      --openstack-designate.reschedule-delay duration               reschedule delay after losing provider
      --openstack-designate.secrets.pool.size int                   Worker pool size for pool secrets of controller openstack-designate (default: 2)
      --openstack-designate.setup int                               number of processors for controller setup
      --openstack-designate.ttl int                                 Default time-to-live for DNS entries
      --plugin-dir string                                           directory containing go plugins
      --pool.resync-period duration                                 default for all controller "pool.resync-period" options
      --pool.size int                                               default for all controller "pool.size" options
      --providers string                                            cluster to look for provider objects
      --providers.disable-deploy-crds                               disable deployment of required crds for cluster provider
      --providers.id string                                         id for cluster provider
      --reschedule-delay duration                                   default for all controller "reschedule-delay" options
      --server-port-http int                                        HTTP server port (serving /healthz, /metrics, ...)
      --service-dns.default.pool.resync-period duration             Period for resynchronization of pool default of controller service-dns (default: 2m0s)
      --service-dns.default.pool.size int                           Worker pool size for pool default of controller service-dns (default: 2)
      --service-dns.dns-class string                                identifier used to differentiate responsible controllers for entries
      --service-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries
      --service-dns.exclude-domains stringArray                     excluded domains
      --service-dns.key string                                      selecting key for annotation
      --service-dns.target-creator-label-name string                label name to store the creator for generated DNS entries
      --service-dns.target-creator-label-value string               label value for creator label
      --service-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation
      --service-dns.target-namespace string                         target namespace for cross cluster generation
      --service-dns.target-owner-id string                          owner id to use for generated DNS entries
      --service-dns.target-realms string                            realm(s) to use for generated DNS entries
      --service-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control
      --service-dns.targets.pool.size int                           Worker pool size for pool targets of controller service-dns (default: 2)
      --setup int                                                   default for all controller "setup" options
      --target string                                               target cluster for dns requests
      --target-creator-label-name string                            default for all controller "target-creator-label-name" options
      --target-creator-label-value string                           default for all controller "target-creator-label-value" options
      --target-name-prefix string                                   default for all controller "target-name-prefix" options
      --target-namespace string                                     default for all controller "target-namespace" options
      --target-owner-id string                                      default for all controller "target-owner-id" options
      --target-realms string                                        default for all controller "target-realms" options
      --target-set-ignore-owners                                    default for all controller "target-set-ignore-owners" options
      --target.disable-deploy-crds                                  disable deployment of required crds for cluster target
      --target.id string                                            id for cluster target
      --ttl int                                                     default for all controller "ttl" options
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
  str = str.replace("cloudflareDns", "cloudflareDNS")
  return str

excluded = {"name", "help", "identifier", "dry-run",
  "cache-dir", "alicloud-dns.cache-dir", "aws-route53.cache-dir", "azure-dns.cache-dir", "google-clouddns.cache-dir", "openstack-designate.cache-dir", "cloudflare-dns.cache-dir"}
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

