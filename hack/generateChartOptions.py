#!/bin/python

# helper script to regenerate helm chart file: partial of charts/external-dns-management/templates/deployment.yaml


import re

options = """
      --accepted-maintainers string                                 accepted maintainer key(s) for crds
      --alicloud-dns.cache-dir string                               Directory to store zone caches (for reload after restart) of controller alicloud-dns
      --alicloud-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache of controller alicloud-dns (default 120)
      --alicloud-dns.default.pool.size int                          Worker pool size for pool default of controller alicloud-dns (default 2)
      --alicloud-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes of controller alicloud-dns
      --alicloud-dns.dns-class string                               Class identifier used to differentiate responsible controllers for entry resources of controller alicloud-dns (default "gardendns")
      --alicloud-dns.dns-delay duration                             delay between two dns reconciliations of controller alicloud-dns (default 10s)
      --alicloud-dns.dns.pool.resync-period duration                Period for resynchronization for pool dns of controller alicloud-dns (default 15m0s)
      --alicloud-dns.dns.pool.size int                              Worker pool size for pool dns of controller alicloud-dns (default 1)
      --alicloud-dns.dry-run                                        just check, don't modify of controller alicloud-dns
      --alicloud-dns.identifier string                              Identifier used to mark DNS entries in DNS system of controller alicloud-dns (default "dnscontroller")
      --alicloud-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller alicloud-dns (default 1)
      --alicloud-dns.pool.resync-period duration                    Period for resynchronization of controller alicloud-dns
      --alicloud-dns.pool.size int                                  Worker pool size of controller alicloud-dns
      --alicloud-dns.providers.pool.resync-period duration          Period for resynchronization for pool providers of controller alicloud-dns (default 10m0s)
      --alicloud-dns.providers.pool.size int                        Worker pool size for pool providers of controller alicloud-dns (default 2)
      --alicloud-dns.ratelimiter.burst int                          number of burst requests for rate limiter of controller alicloud-dns
      --alicloud-dns.ratelimiter.enabled                            enables rate limiter for DNS provider requests of controller alicloud-dns
      --alicloud-dns.ratelimiter.qps int                            maximum requests/queries per second of controller alicloud-dns
      --alicloud-dns.reschedule-delay duration                      reschedule delay after losing provider of controller alicloud-dns (default 2m0s)
      --alicloud-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller alicloud-dns (default 2)
      --alicloud-dns.setup int                                      number of processors for controller setup of controller alicloud-dns (default 10)
      --alicloud-dns.statistic.pool.size int                        Worker pool size for pool statistic of controller alicloud-dns (default 1)
      --alicloud-dns.ttl int                                        Default time-to-live for DNS entries of controller alicloud-dns (default 300)
      --annotation.default.pool.size int                            Worker pool size for pool default of controller annotation (default 5)
      --annotation.pool.size int                                    Worker pool size of controller annotation
      --annotation.setup int                                        number of processors for controller setup of controller annotation (default 10)
      --aws-route53.cache-dir string                                Directory to store zone caches (for reload after restart) of controller aws-route53
      --aws-route53.cache-ttl int                                   Time-to-live for provider hosted zone cache of controller aws-route53 (default 120)
      --aws-route53.default.pool.size int                           Worker pool size for pool default of controller aws-route53 (default 2)
      --aws-route53.disable-zone-state-caching                      disable use of cached dns zone state on changes of controller aws-route53
      --aws-route53.dns-class string                                Class identifier used to differentiate responsible controllers for entry resources of controller aws-route53 (default "gardendns")
      --aws-route53.dns-delay duration                              delay between two dns reconciliations of controller aws-route53 (default 10s)
      --aws-route53.dns.pool.resync-period duration                 Period for resynchronization for pool dns of controller aws-route53 (default 15m0s)
      --aws-route53.dns.pool.size int                               Worker pool size for pool dns of controller aws-route53 (default 1)
      --aws-route53.dry-run                                         just check, don't modify of controller aws-route53
      --aws-route53.identifier string                               Identifier used to mark DNS entries in DNS system of controller aws-route53 (default "dnscontroller")
      --aws-route53.ownerids.pool.size int                          Worker pool size for pool ownerids of controller aws-route53 (default 1)
      --aws-route53.pool.resync-period duration                     Period for resynchronization of controller aws-route53
      --aws-route53.pool.size int                                   Worker pool size of controller aws-route53
      --aws-route53.providers.pool.resync-period duration           Period for resynchronization for pool providers of controller aws-route53 (default 10m0s)
      --aws-route53.providers.pool.size int                         Worker pool size for pool providers of controller aws-route53 (default 2)
      --aws-route53.ratelimiter.burst int                           number of burst requests for rate limiter of controller aws-route53
      --aws-route53.ratelimiter.enabled                             enables rate limiter for DNS provider requests of controller aws-route53
      --aws-route53.ratelimiter.qps int                             maximum requests/queries per second of controller aws-route53
      --aws-route53.reschedule-delay duration                       reschedule delay after losing provider of controller aws-route53 (default 2m0s)
      --aws-route53.secrets.pool.size int                           Worker pool size for pool secrets of controller aws-route53 (default 2)
      --aws-route53.setup int                                       number of processors for controller setup of controller aws-route53 (default 10)
      --aws-route53.statistic.pool.size int                         Worker pool size for pool statistic of controller aws-route53 (default 1)
      --aws-route53.ttl int                                         Default time-to-live for DNS entries of controller aws-route53 (default 300)
      --azure-dns.cache-dir string                                  Directory to store zone caches (for reload after restart) of controller azure-dns
      --azure-dns.cache-ttl int                                     Time-to-live for provider hosted zone cache of controller azure-dns (default 120)
      --azure-dns.default.pool.size int                             Worker pool size for pool default of controller azure-dns (default 2)
      --azure-dns.disable-zone-state-caching                        disable use of cached dns zone state on changes of controller azure-dns
      --azure-dns.dns-class string                                  Class identifier used to differentiate responsible controllers for entry resources of controller azure-dns (default "gardendns")
      --azure-dns.dns-delay duration                                delay between two dns reconciliations of controller azure-dns (default 10s)
      --azure-dns.dns.pool.resync-period duration                   Period for resynchronization for pool dns of controller azure-dns (default 15m0s)
      --azure-dns.dns.pool.size int                                 Worker pool size for pool dns of controller azure-dns (default 1)
      --azure-dns.dry-run                                           just check, don't modify of controller azure-dns
      --azure-dns.identifier string                                 Identifier used to mark DNS entries in DNS system of controller azure-dns (default "dnscontroller")
      --azure-dns.ownerids.pool.size int                            Worker pool size for pool ownerids of controller azure-dns (default 1)
      --azure-dns.pool.resync-period duration                       Period for resynchronization of controller azure-dns
      --azure-dns.pool.size int                                     Worker pool size of controller azure-dns
      --azure-dns.providers.pool.resync-period duration             Period for resynchronization for pool providers of controller azure-dns (default 10m0s)
      --azure-dns.providers.pool.size int                           Worker pool size for pool providers of controller azure-dns (default 2)
      --azure-dns.ratelimiter.burst int                             number of burst requests for rate limiter of controller azure-dns
      --azure-dns.ratelimiter.enabled                               enables rate limiter for DNS provider requests of controller azure-dns
      --azure-dns.ratelimiter.qps int                               maximum requests/queries per second of controller azure-dns
      --azure-dns.reschedule-delay duration                         reschedule delay after losing provider of controller azure-dns (default 2m0s)
      --azure-dns.secrets.pool.size int                             Worker pool size for pool secrets of controller azure-dns (default 2)
      --azure-dns.setup int                                         number of processors for controller setup of controller azure-dns (default 10)
      --azure-dns.statistic.pool.size int                           Worker pool size for pool statistic of controller azure-dns (default 1)
      --azure-dns.ttl int                                           Default time-to-live for DNS entries of controller azure-dns (default 300)
      --bind-address-http string                                    HTTP server bind address
      --cache-dir string                                            Directory to store zone caches (for reload after restart)
      --cache-ttl int                                               Time-to-live for provider hosted zone cache
      --cloudflare-dns.cache-dir string                             Directory to store zone caches (for reload after restart) of controller cloudflare-dns
      --cloudflare-dns.cache-ttl int                                Time-to-live for provider hosted zone cache of controller cloudflare-dns (default 120)
      --cloudflare-dns.default.pool.size int                        Worker pool size for pool default of controller cloudflare-dns (default 2)
      --cloudflare-dns.disable-zone-state-caching                   disable use of cached dns zone state on changes of controller cloudflare-dns
      --cloudflare-dns.dns-class string                             Class identifier used to differentiate responsible controllers for entry resources of controller cloudflare-dns (default "gardendns")
      --cloudflare-dns.dns-delay duration                           delay between two dns reconciliations of controller cloudflare-dns (default 10s)
      --cloudflare-dns.dns.pool.resync-period duration              Period for resynchronization for pool dns of controller cloudflare-dns (default 15m0s)
      --cloudflare-dns.dns.pool.size int                            Worker pool size for pool dns of controller cloudflare-dns (default 1)
      --cloudflare-dns.dry-run                                      just check, don't modify of controller cloudflare-dns
      --cloudflare-dns.identifier string                            Identifier used to mark DNS entries in DNS system of controller cloudflare-dns (default "dnscontroller")
      --cloudflare-dns.ownerids.pool.size int                       Worker pool size for pool ownerids of controller cloudflare-dns (default 1)
      --cloudflare-dns.pool.resync-period duration                  Period for resynchronization of controller cloudflare-dns
      --cloudflare-dns.pool.size int                                Worker pool size of controller cloudflare-dns
      --cloudflare-dns.providers.pool.resync-period duration        Period for resynchronization for pool providers of controller cloudflare-dns (default 10m0s)
      --cloudflare-dns.providers.pool.size int                      Worker pool size for pool providers of controller cloudflare-dns (default 2)
      --cloudflare-dns.ratelimiter.burst int                        number of burst requests for rate limiter of controller cloudflare-dns
      --cloudflare-dns.ratelimiter.enabled                          enables rate limiter for DNS provider requests of controller cloudflare-dns
      --cloudflare-dns.ratelimiter.qps int                          maximum requests/queries per second of controller cloudflare-dns
      --cloudflare-dns.reschedule-delay duration                    reschedule delay after losing provider of controller cloudflare-dns (default 2m0s)
      --cloudflare-dns.secrets.pool.size int                        Worker pool size for pool secrets of controller cloudflare-dns (default 2)
      --cloudflare-dns.setup int                                    number of processors for controller setup of controller cloudflare-dns (default 10)
      --cloudflare-dns.statistic.pool.size int                      Worker pool size for pool statistic of controller cloudflare-dns (default 1)
      --cloudflare-dns.ttl int                                      Default time-to-live for DNS entries of controller cloudflare-dns (default 300)
      --config string                                               config file
  -c, --controllers string                                          comma separated list of controllers to start (<name>,<group>,all) (default "all")
      --cpuprofile string                                           set file for cpu profiling
      --default.pool.resync-period duration                         Period for resynchronization for pool default
      --default.pool.size int                                       Worker pool size for pool default
      --disable-namespace-restriction                               disable access restriction for namespace local access only
      --disable-zone-state-caching                                  disable use of cached dns zone state on changes
      --dns-class string                                            Class identifier used to differentiate responsible controllers for entry resources
      --dns-delay duration                                          delay between two dns reconciliations
      --dns-target-class string                                     identifier used to differentiate responsible dns controllers for target entries
      --dns.pool.resync-period duration                             Period for resynchronization for pool dns
      --dns.pool.size int                                           Worker pool size for pool dns
      --dnsentry-source.default.pool.resync-period duration         Period for resynchronization for pool default of controller dnsentry-source (default 2m0s)
      --dnsentry-source.default.pool.size int                       Worker pool size for pool default of controller dnsentry-source (default 2)
      --dnsentry-source.dns-class string                            identifier used to differentiate responsible controllers for entries of controller dnsentry-source (default "gardendns")
      --dnsentry-source.dns-target-class string                     identifier used to differentiate responsible dns controllers for target entries of controller dnsentry-source
      --dnsentry-source.exclude-domains stringArray                 excluded domains of controller dnsentry-source
      --dnsentry-source.key string                                  selecting key for annotation of controller dnsentry-source
      --dnsentry-source.pool.resync-period duration                 Period for resynchronization of controller dnsentry-source
      --dnsentry-source.pool.size int                               Worker pool size of controller dnsentry-source
      --dnsentry-source.target-creator-label-name string            label name to store the creator for generated DNS entries of controller dnsentry-source (default "creator")
      --dnsentry-source.target-creator-label-value string           label value for creator label of controller dnsentry-source
      --dnsentry-source.target-name-prefix string                   name prefix in target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-namespace string                     target namespace for cross cluster generation of controller dnsentry-source
      --dnsentry-source.target-owner-id string                      owner id to use for generated DNS entries of controller dnsentry-source
      --dnsentry-source.target-realms string                        realm(s) to use for generated DNS entries of controller dnsentry-source
      --dnsentry-source.target-set-ignore-owners                    mark generated DNS entries to omit owner based access control of controller dnsentry-source
      --dnsentry-source.targets.pool.size int                       Worker pool size for pool targets of controller dnsentry-source (default 2)
      --dry-run                                                     just check, don't modify
      --exclude-domains stringArray                                 excluded domains
      --force-crd-update                                            enforce update of crds even they are unmanaged
      --google-clouddns.cache-dir string                            Directory to store zone caches (for reload after restart) of controller google-clouddns
      --google-clouddns.cache-ttl int                               Time-to-live for provider hosted zone cache of controller google-clouddns (default 120)
      --google-clouddns.default.pool.size int                       Worker pool size for pool default of controller google-clouddns (default 2)
      --google-clouddns.disable-zone-state-caching                  disable use of cached dns zone state on changes of controller google-clouddns
      --google-clouddns.dns-class string                            Class identifier used to differentiate responsible controllers for entry resources of controller google-clouddns (default "gardendns")
      --google-clouddns.dns-delay duration                          delay between two dns reconciliations of controller google-clouddns (default 10s)
      --google-clouddns.dns.pool.resync-period duration             Period for resynchronization for pool dns of controller google-clouddns (default 15m0s)
      --google-clouddns.dns.pool.size int                           Worker pool size for pool dns of controller google-clouddns (default 1)
      --google-clouddns.dry-run                                     just check, don't modify of controller google-clouddns
      --google-clouddns.identifier string                           Identifier used to mark DNS entries in DNS system of controller google-clouddns (default "dnscontroller")
      --google-clouddns.ownerids.pool.size int                      Worker pool size for pool ownerids of controller google-clouddns (default 1)
      --google-clouddns.pool.resync-period duration                 Period for resynchronization of controller google-clouddns
      --google-clouddns.pool.size int                               Worker pool size of controller google-clouddns
      --google-clouddns.providers.pool.resync-period duration       Period for resynchronization for pool providers of controller google-clouddns (default 10m0s)
      --google-clouddns.providers.pool.size int                     Worker pool size for pool providers of controller google-clouddns (default 2)
      --google-clouddns.ratelimiter.burst int                       number of burst requests for rate limiter of controller google-clouddns
      --google-clouddns.ratelimiter.enabled                         enables rate limiter for DNS provider requests of controller google-clouddns
      --google-clouddns.ratelimiter.qps int                         maximum requests/queries per second of controller google-clouddns
      --google-clouddns.reschedule-delay duration                   reschedule delay after losing provider of controller google-clouddns (default 2m0s)
      --google-clouddns.secrets.pool.size int                       Worker pool size for pool secrets of controller google-clouddns (default 2)
      --google-clouddns.setup int                                   number of processors for controller setup of controller google-clouddns (default 10)
      --google-clouddns.statistic.pool.size int                     Worker pool size for pool statistic of controller google-clouddns (default 1)
      --google-clouddns.ttl int                                     Default time-to-live for DNS entries of controller google-clouddns (default 300)
      --grace-period duration                                       inactivity grace period for detecting end of cleanup for shutdown
  -h, --help                                                        help for dns-controller-manager
      --identifier string                                           Identifier used to mark DNS entries in DNS system
      --infoblox-dns.cache-dir string                               Directory to store zone caches (for reload after restart) of controller infoblox-dns
      --infoblox-dns.cache-ttl int                                  Time-to-live for provider hosted zone cache of controller infoblox-dns (default 120)
      --infoblox-dns.default.pool.size int                          Worker pool size for pool default of controller infoblox-dns (default 2)
      --infoblox-dns.disable-zone-state-caching                     disable use of cached dns zone state on changes of controller infoblox-dns
      --infoblox-dns.dns-class string                               Class identifier used to differentiate responsible controllers for entry resources of controller infoblox-dns (default "gardendns")
      --infoblox-dns.dns-delay duration                             delay between two dns reconciliations of controller infoblox-dns (default 10s)
      --infoblox-dns.dns.pool.resync-period duration                Period for resynchronization for pool dns of controller infoblox-dns (default 15m0s)
      --infoblox-dns.dns.pool.size int                              Worker pool size for pool dns of controller infoblox-dns (default 1)
      --infoblox-dns.dry-run                                        just check, don't modify of controller infoblox-dns
      --infoblox-dns.identifier string                              Identifier used to mark DNS entries in DNS system of controller infoblox-dns (default "dnscontroller")
      --infoblox-dns.ownerids.pool.size int                         Worker pool size for pool ownerids of controller infoblox-dns (default 1)
      --infoblox-dns.pool.resync-period duration                    Period for resynchronization of controller infoblox-dns
      --infoblox-dns.pool.size int                                  Worker pool size of controller infoblox-dns
      --infoblox-dns.providers.pool.resync-period duration          Period for resynchronization for pool providers of controller infoblox-dns (default 10m0s)
      --infoblox-dns.providers.pool.size int                        Worker pool size for pool providers of controller infoblox-dns (default 2)
      --infoblox-dns.ratelimiter.burst int                          number of burst requests for rate limiter of controller infoblox-dns
      --infoblox-dns.ratelimiter.enabled                            enables rate limiter for DNS provider requests of controller infoblox-dns
      --infoblox-dns.ratelimiter.qps int                            maximum requests/queries per second of controller infoblox-dns
      --infoblox-dns.reschedule-delay duration                      reschedule delay after losing provider of controller infoblox-dns (default 2m0s)
      --infoblox-dns.secrets.pool.size int                          Worker pool size for pool secrets of controller infoblox-dns (default 2)
      --infoblox-dns.setup int                                      number of processors for controller setup of controller infoblox-dns (default 10)
      --infoblox-dns.statistic.pool.size int                        Worker pool size for pool statistic of controller infoblox-dns (default 1)
      --infoblox-dns.ttl int                                        Default time-to-live for DNS entries of controller infoblox-dns (default 300)
      --ingress-dns.default.pool.resync-period duration             Period for resynchronization for pool default of controller ingress-dns (default 2m0s)
      --ingress-dns.default.pool.size int                           Worker pool size for pool default of controller ingress-dns (default 2)
      --ingress-dns.dns-class string                                identifier used to differentiate responsible controllers for entries of controller ingress-dns (default "gardendns")
      --ingress-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries of controller ingress-dns
      --ingress-dns.exclude-domains stringArray                     excluded domains of controller ingress-dns
      --ingress-dns.key string                                      selecting key for annotation of controller ingress-dns
      --ingress-dns.pool.resync-period duration                     Period for resynchronization of controller ingress-dns
      --ingress-dns.pool.size int                                   Worker pool size of controller ingress-dns
      --ingress-dns.target-creator-label-name string                label name to store the creator for generated DNS entries of controller ingress-dns (default "creator")
      --ingress-dns.target-creator-label-value string               label value for creator label of controller ingress-dns
      --ingress-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-namespace string                         target namespace for cross cluster generation of controller ingress-dns
      --ingress-dns.target-owner-id string                          owner id to use for generated DNS entries of controller ingress-dns
      --ingress-dns.target-realms string                            realm(s) to use for generated DNS entries of controller ingress-dns
      --ingress-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control of controller ingress-dns
      --ingress-dns.targets.pool.size int                           Worker pool size for pool targets of controller ingress-dns (default 2)
      --key string                                                  selecting key for annotation
      --kubeconfig string                                           default cluster access
      --kubeconfig.disable-deploy-crds                              disable deployment of required crds for cluster default
      --kubeconfig.id string                                        id for cluster default
      --lease-duration duration                                     lease duration (default 15s)
      --lease-name string                                           name for lease object
      --lease-renew-deadline duration                               lease renew deadline (default 10s)
      --lease-retry-period duration                                 lease retry period (default 2s)
  -D, --log-level string                                            logrus log level
      --maintainer string                                           maintainer key for crds (default "dns-controller-manager")
      --name string                                                 name used for controller manager (default "dns-controller-manager")
      --namespace string                                            namespace for lease (default "kube-system")
  -n, --namespace-local-access-only                                 enable access restriction for namespace local access only (deprecated)
      --omit-lease                                                  omit lease for development
      --openstack-designate.cache-dir string                        Directory to store zone caches (for reload after restart) of controller openstack-designate
      --openstack-designate.cache-ttl int                           Time-to-live for provider hosted zone cache of controller openstack-designate (default 120)
      --openstack-designate.default.pool.size int                   Worker pool size for pool default of controller openstack-designate (default 2)
      --openstack-designate.disable-zone-state-caching              disable use of cached dns zone state on changes of controller openstack-designate
      --openstack-designate.dns-class string                        Class identifier used to differentiate responsible controllers for entry resources of controller openstack-designate (default "gardendns")
      --openstack-designate.dns-delay duration                      delay between two dns reconciliations of controller openstack-designate (default 10s)
      --openstack-designate.dns.pool.resync-period duration         Period for resynchronization for pool dns of controller openstack-designate (default 15m0s)
      --openstack-designate.dns.pool.size int                       Worker pool size for pool dns of controller openstack-designate (default 1)
      --openstack-designate.dry-run                                 just check, don't modify of controller openstack-designate
      --openstack-designate.identifier string                       Identifier used to mark DNS entries in DNS system of controller openstack-designate (default "dnscontroller")
      --openstack-designate.ownerids.pool.size int                  Worker pool size for pool ownerids of controller openstack-designate (default 1)
      --openstack-designate.pool.resync-period duration             Period for resynchronization of controller openstack-designate
      --openstack-designate.pool.size int                           Worker pool size of controller openstack-designate
      --openstack-designate.providers.pool.resync-period duration   Period for resynchronization for pool providers of controller openstack-designate (default 10m0s)
      --openstack-designate.providers.pool.size int                 Worker pool size for pool providers of controller openstack-designate (default 2)
      --openstack-designate.ratelimiter.burst int                   number of burst requests for rate limiter of controller openstack-designate
      --openstack-designate.ratelimiter.enabled                     enables rate limiter for DNS provider requests of controller openstack-designate
      --openstack-designate.ratelimiter.qps int                     maximum requests/queries per second of controller openstack-designate
      --openstack-designate.reschedule-delay duration               reschedule delay after losing provider of controller openstack-designate (default 2m0s)
      --openstack-designate.secrets.pool.size int                   Worker pool size for pool secrets of controller openstack-designate (default 2)
      --openstack-designate.setup int                               number of processors for controller setup of controller openstack-designate (default 10)
      --openstack-designate.statistic.pool.size int                 Worker pool size for pool statistic of controller openstack-designate (default 1)
      --openstack-designate.ttl int                                 Default time-to-live for DNS entries of controller openstack-designate (default 300)
      --ownerids.pool.size int                                      Worker pool size for pool ownerids
      --plugin-file string                                          directory containing go plugins
      --pool.resync-period duration                                 Period for resynchronization
      --pool.size int                                               Worker pool size
      --providers string                                            cluster to look for provider objects
      --providers.disable-deploy-crds                               disable deployment of required crds for cluster provider
      --providers.id string                                         id for cluster provider
      --providers.pool.resync-period duration                       Period for resynchronization for pool providers
      --providers.pool.size int                                     Worker pool size for pool providers
      --ratelimiter.burst int                                       number of burst requests for rate limiter
      --ratelimiter.enabled                                         enables rate limiter for DNS provider requests
      --ratelimiter.qps int                                         maximum requests/queries per second
      --reschedule-delay duration                                   reschedule delay after losing provider
      --secrets.pool.size int                                       Worker pool size for pool secrets
      --server-port-http int                                        HTTP server port (serving /healthz, /metrics, ...)
      --service-dns.default.pool.resync-period duration             Period for resynchronization for pool default of controller service-dns (default 2m0s)
      --service-dns.default.pool.size int                           Worker pool size for pool default of controller service-dns (default 2)
      --service-dns.dns-class string                                identifier used to differentiate responsible controllers for entries of controller service-dns (default "gardendns")
      --service-dns.dns-target-class string                         identifier used to differentiate responsible dns controllers for target entries of controller service-dns
      --service-dns.exclude-domains stringArray                     excluded domains of controller service-dns
      --service-dns.key string                                      selecting key for annotation of controller service-dns
      --service-dns.pool.resync-period duration                     Period for resynchronization of controller service-dns
      --service-dns.pool.size int                                   Worker pool size of controller service-dns
      --service-dns.target-creator-label-name string                label name to store the creator for generated DNS entries of controller service-dns (default "creator")
      --service-dns.target-creator-label-value string               label value for creator label of controller service-dns
      --service-dns.target-name-prefix string                       name prefix in target namespace for cross cluster generation of controller service-dns
      --service-dns.target-namespace string                         target namespace for cross cluster generation of controller service-dns
      --service-dns.target-owner-id string                          owner id to use for generated DNS entries of controller service-dns
      --service-dns.target-realms string                            realm(s) to use for generated DNS entries of controller service-dns
      --service-dns.target-set-ignore-owners                        mark generated DNS entries to omit owner based access control of controller service-dns
      --service-dns.targets.pool.size int                           Worker pool size for pool targets of controller service-dns (default 2)
      --setup int                                                   number of processors for controller setup
      --statistic.pool.size int                                     Worker pool size for pool statistic
      --target string                                               target cluster for dns requests
      --target-creator-label-name string                            label name to store the creator for generated DNS entries
      --target-creator-label-value string                           label value for creator label
      --target-name-prefix string                                   name prefix in target namespace for cross cluster generation
      --target-namespace string                                     target namespace for cross cluster generation
      --target-owner-id string                                      owner id to use for generated DNS entries
      --target-realms string                                        realm(s) to use for generated DNS entries
      --target-set-ignore-owners                                    mark generated DNS entries to omit owner based access control
      --target.disable-deploy-crds                                  disable deployment of required crds for cluster target
      --target.id string                                            id for cluster target
      --targets.pool.size int                                       Worker pool size for pool targets
      --ttl int                                                     Default time-to-live for DNS entries
      --version                                                     version for dns-controller-manager
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

