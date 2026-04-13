<p>Packages:</p>
<ul>
<li>
<a href="#config.dns.gardener.cloud%2fv1alpha1">config.dns.gardener.cloud/v1alpha1</a>
</li>
</ul>

<h2 id="config.dns.gardener.cloud/v1alpha1">config.dns.gardener.cloud/v1alpha1</h2>
<p>

</p>

<h3 id="advancedoptions">AdvancedOptions
</h3>


<p>
(<em>Appears on:</em><a href="#dnsmanagerconfiguration">DNSManagerConfiguration</a>)
</p>

<p>
AdvancedOptions contains advanced options for a DNS provider type.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>rateLimits</code></br>
<em>
<a href="#ratelimiteroptions">RateLimiterOptions</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RateLimits contains the rate limiter configuration for the provider.</p>
</td>
</tr>
<tr>
<td>
<code>batchSize</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>BatchSize is the batch size for change requests (currently only used for aws-route53).</p>
</td>
</tr>
<tr>
<td>
<code>maxRetries</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxRetries is the maximum number of retries to avoid paging stops on throttling (currently only used for aws-route53).</p>
</td>
</tr>
<tr>
<td>
<code>blockedZones</code></br>
<em>
string array
</em>
</td>
<td>
<em>(Optional)</em>
<p>BlockedZones is a list of zone IDs that are blocked from being used by the provider.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="clientconnection">ClientConnection
</h3>


<p>
(<em>Appears on:</em><a href="#dnsmanagerconfiguration">DNSManagerConfiguration</a>)
</p>

<p>
ClientConnection contains client connection configurations
for the primary cluster (certificates and source resources).
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>ClientConnectionConfiguration</code></br>
<em>
<a href="#clientconnectionconfiguration">ClientConnectionConfiguration</a>
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>cacheResyncPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<p>CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="controlplaneclientconnection">ControlPlaneClientConnection
</h3>


<p>
(<em>Appears on:</em><a href="#dnsmanagerconfiguration">DNSManagerConfiguration</a>)
</p>

<p>
ControlPlaneClientConnection contains client connection configurations
for the cluster containing the provided issuers.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>ClientConnectionConfiguration</code></br>
<em>
<a href="#clientconnectionconfiguration">ClientConnectionConfiguration</a>
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>cacheResyncPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<p>CacheResyncPeriod specifies the duration how often the cache for the cluster is resynced.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="controllerconfiguration">ControllerConfiguration
</h3>


<p>
(<em>Appears on:</em><a href="#dnsmanagerconfiguration">DNSManagerConfiguration</a>)
</p>

<p>
ControllerConfiguration defines the configuration of the controllers.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>dnsProvider</code></br>
<em>
<a href="#dnsprovidercontrollerconfig">DNSProviderControllerConfig</a>
</em>
</td>
<td>
<p>DNSProvider is the configuration for the DNSProvider controller.</p>
</td>
</tr>
<tr>
<td>
<code>dnsEntry</code></br>
<em>
<a href="#dnsentrycontrollerconfig">DNSEntryControllerConfig</a>
</em>
</td>
<td>
<p>DNSEntry is the configuration for the DNSEntry controller.</p>
</td>
</tr>
<tr>
<td>
<code>dnsAnnotation</code></br>
<em>
<a href="#dnsannotationcontrollerconfig">DNSAnnotationControllerConfig</a>
</em>
</td>
<td>
<p>DNSAnnotation is the configuration for the DNSAnnotation controller.</p>
</td>
</tr>
<tr>
<td>
<code>source</code></br>
<em>
<a href="#sourcecontrollerconfig">SourceControllerConfig</a>
</em>
</td>
<td>
<p>Source is the common configuration for source controllers.</p>
</td>
</tr>
<tr>
<td>
<code>skipNameValidation</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>SkipNameValidation if true, the controller registration will skip the validation of its names in the controller runtime.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsannotationcontrollerconfig">DNSAnnotationControllerConfig
</h3>


<p>
(<em>Appears on:</em><a href="#controllerconfiguration">ControllerConfiguration</a>)
</p>

<p>
DNSAnnotationControllerConfig is the configuration for the DNSAnnotation controller.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>concurrentSyncs</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConcurrentSyncs is the number of concurrent reconciliations for this controller.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsentrycontrollerconfig">DNSEntryControllerConfig
</h3>


<p>
(<em>Appears on:</em><a href="#controllerconfiguration">ControllerConfiguration</a>)
</p>

<p>
DNSEntryControllerConfig is the configuration for the DNSEntry controller.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>concurrentSyncs</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConcurrentSyncs is the number of concurrent worker routines for this controller.</p>
</td>
</tr>
<tr>
<td>
<code>syncPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SyncPeriod is the periodic reconciliation interval for all DNSEntry objects.</p>
</td>
</tr>
<tr>
<td>
<code>reconciliationTimeout</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReconciliationTimeout is the maximum duration a reconciliation of a DNSEntry is allowed to take.<br />Default value is 2 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>maxConcurrentLookups</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxConcurrentLookups is the number of concurrent DNS lookups for the lookup processor.</p>
</td>
</tr>
<tr>
<td>
<code>defaultCNAMELookupInterval</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>DefaultCNAMELookupInterval is the default interval for CNAME lookups in seconds.</p>
</td>
</tr>
<tr>
<td>
<code>reconciliationDelayAfterUpdate</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReconciliationDelayAfterUpdate is the duration to wait after a DNSEntry object has been updated before its reconciliation is performed.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsmanagerconfiguration">DNSManagerConfiguration
</h3>


<p>
DNSManagerConfiguration defines the configuration for the Gardener dns-controller-manager.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>clientConnection</code></br>
<em>
<a href="#clientconnection">ClientConnection</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ClientConnection specifies the kubeconfig file and the client connection settings for primary<br />cluster containing the source resources the dns-controller-manager should work on.</p>
</td>
</tr>
<tr>
<td>
<code>controlPlaneClientConnection</code></br>
<em>
<a href="#controlplaneclientconnection">ControlPlaneClientConnection</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ControlPlaneClientConnection contains client connection configurations<br />for the cluster containing the provided DNSProviders and target DNSEntries.<br />If not set, the primary cluster is used.</p>
</td>
</tr>
<tr>
<td>
<code>leaderElection</code></br>
<em>
<a href="#leaderelectionconfiguration">LeaderElectionConfiguration</a>
</em>
</td>
<td>
<p>LeaderElection defines the configuration of leader election client.</p>
</td>
</tr>
<tr>
<td>
<code>logLevel</code></br>
<em>
string
</em>
</td>
<td>
<p>LogLevel is the level/severity for the logs. Must be one of [info,debug,error].</p>
</td>
</tr>
<tr>
<td>
<code>logFormat</code></br>
<em>
string
</em>
</td>
<td>
<p>LogFormat is the output format for the logs. Must be one of [text,json].</p>
</td>
</tr>
<tr>
<td>
<code>server</code></br>
<em>
<a href="#serverconfiguration">ServerConfiguration</a>
</em>
</td>
<td>
<p>Server defines the configuration of the HTTP server.</p>
</td>
</tr>
<tr>
<td>
<code>debugging</code></br>
<em>
<a href="#debuggingconfiguration">DebuggingConfiguration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Debugging holds configuration for Debugging related features.</p>
</td>
</tr>
<tr>
<td>
<code>controllers</code></br>
<em>
<a href="#controllerconfiguration">ControllerConfiguration</a>
</em>
</td>
<td>
<p>Controllers defines the configuration of the controllers.</p>
</td>
</tr>
<tr>
<td>
<code>class</code></br>
<em>
string
</em>
</td>
<td>
<p>Class is the "dns.gardener.cloud/class" the dns-controller-manager is responsible for.<br />If not set, the default class "gardendns" is used.</p>
</td>
</tr>
<tr>
<td>
<code>deployCRDs</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeployCRDs indicates whether the required CRDs should be deployed to the main cluster on startup.<br />This does not include the control plane cluster, if different.</p>
</td>
</tr>
<tr>
<td>
<code>conditionalDeployCRDs</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConditionalDeployCRDs indicates whether to check before deploying CRDs if there is a managed resource in the garden namespace managing it.</p>
</td>
</tr>
<tr>
<td>
<code>addShootNoCleanupLabelToCRDs</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>AddShootNoCleanupLabelToCRDs indicates whether to add the "shoot.gardener.cloud/no-cleanup" label to deployed CRDs.<br />This prevents Gardener from cleaning them up when the shoot is deleted.</p>
</td>
</tr>
<tr>
<td>
<code>providerAdvancedOptions</code></br>
<em>
object (keys:string, values:<a href="#advancedoptions">AdvancedOptions</a>)
</em>
</td>
<td>
<em>(Optional)</em>
<p>ProviderAdvancedOptions contains advanced options for the DNS provider types.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="dnsprovidercontrollerconfig">DNSProviderControllerConfig
</h3>


<p>
(<em>Appears on:</em><a href="#controllerconfiguration">ControllerConfiguration</a>)
</p>

<p>
DNSProviderControllerConfig is the configuration for the DNSProvider controller.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>concurrentSyncs</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConcurrentSyncs is the number of concurrent worker routines for this controller.</p>
</td>
</tr>
<tr>
<td>
<code>syncPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SyncPeriod is the periodic reconciliation interval for all DNSProvider objects.<br />Default is 1 hour.</p>
</td>
</tr>
<tr>
<td>
<code>recheckPeriod</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RecheckPeriod is the duration how often the controller rechecks a provider on a recoverable error.<br />Default value is 5 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>reconciliationTimeout</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReconciliationTimeout is the maximum duration a reconciliation of a DNSProvider is allowed to take.<br />Default value is 2 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code></br>
<em>
string
</em>
</td>
<td>
<p>Namespace is the namespace on the secondary cluster containing the provided DNSProviders.</p>
</td>
</tr>
<tr>
<td>
<code>enabledProviderTypes</code></br>
<em>
string array
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnabledProviderTypes is the list of DNS provider types that should be enabled.<br />If not set, all provider types are enabled.</p>
</td>
</tr>
<tr>
<td>
<code>disabledProviderTypes</code></br>
<em>
string array
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisabledProviderTypes is the list of DNS provider types that should be disabled.<br />If not set, no provider types are disabled.</p>
</td>
</tr>
<tr>
<td>
<code>defaultRateLimits</code></br>
<em>
<a href="#ratelimiteroptions">RateLimiterOptions</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DefaultRateLimits defines the rate limiter configuration for a DNSProvider account if not overridden by the DNSProvider.</p>
</td>
</tr>
<tr>
<td>
<code>defaultTTL</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>DefaultTTL is the default TTL used for DNS entries if not specified explicitly. May be overridden by the DNSProvider.</p>
</td>
</tr>
<tr>
<td>
<code>zoneCacheTTL</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.33/#duration-v1-meta">Duration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ZoneCacheTTL is the TTL for caching provider zones.<br />Default is 30 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>migrationMode</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>MigrationMode if true, the controller runs in migration mode and will not add finalizers to secrets.<br />This is useful when migrating if an old controller is still running on the control plane cluster for other DNS classes.</p>
</td>
</tr>
<tr>
<td>
<code>gcpWorkloadIdentityConfig</code></br>
<em>
<a href="#gcpworkloadidentityconfig">GCPWorkloadIdentityConfig</a>
</em>
</td>
<td>
<p>GCPWorkloadIdentityConfig is the configuration for GCP workload identity.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="gcpworkloadidentityconfig">GCPWorkloadIdentityConfig
</h3>


<p>
(<em>Appears on:</em><a href="#dnsprovidercontrollerconfig">DNSProviderControllerConfig</a>)
</p>

<p>
GCPWorkloadIdentityConfig is the configuration for GCP workload identity.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>allowedTokenURLs</code></br>
<em>
string array
</em>
</td>
<td>
<p>AllowedTokenURLs are the allowed token URLs.</p>
</td>
</tr>
<tr>
<td>
<code>allowedServiceAccountImpersonationURLRegExps</code></br>
<em>
string array
</em>
</td>
<td>
<p>AllowedServiceAccountImpersonationURLRegExps are the allowed service account impersonation URL regular expressions.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="ratelimiteroptions">RateLimiterOptions
</h3>


<p>
(<em>Appears on:</em><a href="#advancedoptions">AdvancedOptions</a>, <a href="#dnsprovidercontrollerconfig">DNSProviderControllerConfig</a>)
</p>

<p>
RateLimiterOptions defines the rate limiter configuration.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>enabled</code></br>
<em>
boolean
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>qps</code></br>
<em>
float
</em>
</td>
<td>
<p></p>
</td>
</tr>
<tr>
<td>
<code>burst</code></br>
<em>
integer
</em>
</td>
<td>
<p></p>
</td>
</tr>

</tbody>
</table>


<h3 id="server">Server
</h3>


<p>
(<em>Appears on:</em><a href="#serverconfiguration">ServerConfiguration</a>)
</p>

<p>
Server contains information for HTTP(S) server configuration.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>bindAddress</code></br>
<em>
string
</em>
</td>
<td>
<p>BindAddress is the IP address on which to listen for the specified port.</p>
</td>
</tr>
<tr>
<td>
<code>port</code></br>
<em>
integer
</em>
</td>
<td>
<p>Port is the port on which to serve requests.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="serverconfiguration">ServerConfiguration
</h3>


<p>
(<em>Appears on:</em><a href="#dnsmanagerconfiguration">DNSManagerConfiguration</a>)
</p>

<p>
ServerConfiguration contains details for the HTTP(S) servers.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>webhooks</code></br>
<em>
<a href="#server">Server</a>
</em>
</td>
<td>
<p>Webhooks is the configuration for the HTTPS webhook server.</p>
</td>
</tr>
<tr>
<td>
<code>healthProbes</code></br>
<em>
<a href="#server">Server</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HealthProbes is the configuration for serving the healthz and readyz endpoints.</p>
</td>
</tr>
<tr>
<td>
<code>metrics</code></br>
<em>
<a href="#server">Server</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Metrics is the configuration for serving the metrics endpoint.</p>
</td>
</tr>

</tbody>
</table>


<h3 id="sourcecontrollerconfig">SourceControllerConfig
</h3>


<p>
(<em>Appears on:</em><a href="#controllerconfiguration">ControllerConfiguration</a>)
</p>

<p>
SourceControllerConfig is the configuration for the source controllers.
</p>

<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>

<tr>
<td>
<code>concurrentSyncs</code></br>
<em>
integer
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConcurrentSyncs is the number of concurrent reconciliations for source controllers.</p>
</td>
</tr>
<tr>
<td>
<code>sourceClass</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SourceClass is the class value for sources.</p>
</td>
</tr>
<tr>
<td>
<code>targetClass</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetClass is the class value for target DNSEntries.</p>
</td>
</tr>
<tr>
<td>
<code>targetNamespace</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetNamespace is the namespace for target DNSEntries.</p>
</td>
</tr>
<tr>
<td>
<code>targetNamePrefix</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetNamePrefix is the prefix for target DNSEntries object names.</p>
</td>
</tr>
<tr>
<td>
<code>targetLabels</code></br>
<em>
object (keys:string, values:string)
</em>
</td>
<td>
<p>TargetLabels are the labels to be added to target DNSEntries and DNSProviders.</p>
</td>
</tr>
<tr>
<td>
<code>targetClusterID</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetClusterID is the cluster ID of the target cluster.</p>
</td>
</tr>
<tr>
<td>
<code>sourceClusterID</code></br>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SourceClusterID is the cluster ID of the source cluster.</p>
</td>
</tr>
<tr>
<td>
<code>dnsProviderReplication</code></br>
<em>
boolean
</em>
</td>
<td>
<em>(Optional)</em>
<p>DNSProviderReplication indicates whether DNSProvider replication from source to target cluster is enabled.</p>
</td>
</tr>

</tbody>
</table>


