/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const A_MAINTAINER = "crds.gardener.cloud/maintainer"

type EventRecorder interface {
	// The resulting event will be created in the same namespace as the reference object.
	// Event(object runtime.ObjectData, eventtype, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	// Eventf(object runtime.ObjectData, eventtype, reason, messageFmt string, args ...interface{})

	// AnnotatedEventf is just like eventf, but with annotations attached
	// AnnotatedEventf(object runtime.ObjectData, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})
}

type ReconcilationElementSpec interface {
	String() string
}

var _ ReconcilationElementSpec = schema.GroupKind{}
var _ ReconcilationElementSpec = utils.Matcher(nil)

type _ReconcilationKey struct {
	key        ReconcilationElementSpec
	cluster    string
	reconciler string
}

type _Reconcilations map[_ReconcilationKey]string

func (this _Reconcilations) Get(cluster resources.Cluster, gk schema.GroupKind) utils.StringSet {
	reconcilers := utils.StringSet{}
	cluster_name := cluster.GetName()
	for k := range this {
		if k.cluster == cluster_name && k.key == gk {
			reconcilers.Add(k.reconciler)
		}
	}
	return reconcilers
}

type ReadyFlag struct {
	lock    sync.Mutex
	isready bool
}

func (this *ReadyFlag) WhenReady() {
	this.lock.Lock()
	this.lock.Unlock()
}

func (this *ReadyFlag) IsReady() bool {
	return this.isready
}

func (this *ReadyFlag) ready() {
	this.isready = true
	this.lock.Unlock()
}

func (this *ReadyFlag) start() {
	this.lock.Lock()
}

type controller struct {
	extension.ElementBase
	record.EventRecorder

	sharedAttributes

	ready           ReadyFlag
	definition      Definition
	env             Environment
	cluster         cluster.Interface
	clusters        cluster.Clusters
	filters         []ResourceFilter
	owning          WatchResource
	watches         map[string][]Watch
	reconcilers     map[string]reconcile.Interface
	reconcilerNames map[reconcile.Interface]string
	mappings        _Reconcilations
	syncRequests    *SyncRequests
	finalizer       Finalizer

	options  *ControllerConfig
	handlers map[string]*ClusterHandler

	pools map[string]*pool
}

func Filter(owning ResourceKey, resc resources.Object) bool {
	return true
}

func NewController(env Environment, def Definition, cmp mappings.Definition) (*controller, error) {
	options := env.GetConfig().GetSource(def.Name()).(*ControllerConfig)

	this := &controller{
		definition: def,
		options:    options,
		env:        env,

		owning:  def.MainWatchResource(),
		filters: def.ResourceFilters(),

		handlers:        map[string]*ClusterHandler{},
		watches:         map[string][]Watch{},
		pools:           map[string]*pool{},
		reconcilers:     map[string]reconcile.Interface{},
		reconcilerNames: map[reconcile.Interface]string{},
		mappings:        _Reconcilations{},
		finalizer:       NewDefaultFinalizer(def.FinalizerName()),
	}

	this.syncRequests = NewSyncRequests(this)

	ctx := ctxutil.WaitGroupContext(env.GetContext(), "controller ", def.Name())
	this.ElementBase = extension.NewElementBase(ctx, ctx_controller, this, def.Name(), options)
	this.sharedAttributes.LogContext = this.ElementBase
	this.ready.start()

	required := cluster.Canonical(def.RequiredClusters())
	clusters, err := mappings.MapClusters(env.GetClusters(), cmp, required...)
	if err != nil {
		return nil, err
	}
	this.Infof("  using clusters %+v: %s (selected from %s)", required, clusters, env.GetClusters())
	if def.Scheme() != nil {
		if def.Scheme() != resources.DefaultScheme() {
			this.Infof("  using dedicated scheme for clusters")
		}
		clusters, err = clusters.WithScheme(def.Scheme())
		if err != nil {
			return nil, err
		}
	}
	this.clusters = clusters
	this.cluster = clusters.GetCluster(required[0])
	this.EventRecorder = this.cluster.Resources()

	err = this.deployCRDS()
	if err != nil {
		return nil, err
	}

	for n, t := range def.Reconcilers() {
		this.Infof("creating reconciler %q", n)
		reconciler, err := t(this)
		if err != nil {
			return nil, fmt.Errorf("creating reconciler %s failed: %s", n, err)
		}
		this.reconcilers[n] = reconciler
		this.reconcilerNames[reconciler] = n
	}

	for cname, watches := range this.definition.Watches() {
		for _, w := range watches {
			ok, err := this.addReconciler(cname, w.ResourceType().GroupKind(), w.PoolName(), w.Reconciler())
			if err != nil {
				this.Errorf("GOT error: %s", err)
				return nil, err
			}
			if ok {
				this.watches[cname] = append(this.watches[cname], w)
			}
		}
	}
	_, err = this.addReconciler(required[0], this.Owning().GroupKind(), DEFAULT_POOL, DEFAULT_RECONCILER)
	if err != nil {
		return nil, err
	}

	for _, cmds := range this.GetDefinition().Commands() {
		for _, cmd := range cmds {
			_, err := this.addReconciler("", cmd.Key(), cmd.PoolName(), cmd.Reconciler())
			if err != nil {
				return nil, fmt.Errorf("Add matcher for reconciler %s failed: %s", cmd.Reconciler(), err)
			}
		}
	}
	for _, s := range this.GetDefinition().Syncers() {
		cluster := clusters.GetCluster(s.GetCluster())
		reconcilers := this.mappings.Get(cluster, s.GetResource().GroupKind())
		if len(reconcilers) == 0 {
			return nil, fmt.Errorf("resource %q not watched for cluster %s", s.GetResource(), s.GetCluster())
		}
		this.syncRequests.AddSyncer(NewSyncer(s.GetName(), s.GetResource(), cluster))
		this.Infof("adding syncer %s for resource %s on cluster %s", s.GetName(), s.GetResource(), cluster)
	}

	return this, nil
}

func (this *controller) deployImplicitCustomResourceDefinitions(log logger.LogContext, eff WatchedResources, gks resources.GroupKindSet, cluster cluster.Interface) error {
	for gk := range gks {
		if eff.Contains(cluster.GetId(), gk) {
			eff.Remove(cluster.GetId(), gk)
			v, err := apiextensions.NewDefaultedCustomResourceDefinitionVersions(gk)
			if err == nil {
				err := v.Deploy(log, cluster, this.env.ControllerManager().GetMaintainer())
				if err != nil {
					return err
				}
			}
		} else {
			log.Infof("crd for %s already handled", gk)
		}
	}
	return nil
}

func (this *controller) deployCRDS() error {
	// first gather all intended or required resources
	// by its effective and logical cluster usage
	clusterResources := WatchedResources{}.Add(CLUSTER_MAIN, this.Owning().GroupKind())
	effClusterResources := WatchedResources{}.Add(this.GetMainCluster().GetId(), this.Owning().GroupKind())
	for cname, watches := range this.definition.Watches() {
		cluster := this.GetCluster(cname)
		if cluster == nil {
			return fmt.Errorf("cluster %q not found for resource definitions", cname)
		}
		for _, w := range watches {
			clusterResources.Add(cname, w.ResourceType().GroupKind())
			effClusterResources.Add(cluster.GetId(), w.ResourceType().GroupKind())
		}
	}
	for n, crds := range this.definition.CustomResourceDefinitions() {
		cluster := this.GetCluster(n)
		if cluster == nil {
			return fmt.Errorf("cluster %q not found for resource definitions", n)
		}
		for _, v := range crds {
			effClusterResources.Add(cluster.GetId(), v.GroupKind())
		}
	}

	// now deploy explicit requested CRDs or implicitly available CRDs for used resources
	log := this.AddIndent("  ")
	for n, crds := range this.definition.CustomResourceDefinitions() {
		cluster := this.GetCluster(n)
		if isDeployCRDsDisabled(cluster) {
			this.Infof("deployment of required crds is disabled for cluster %q (used for %q)", cluster.GetName(), n)
			continue
		}
		this.Infof("ensure required crds for cluster %q (used for %q)", cluster.GetName(), n)
		for _, v := range crds {
			clusterResources.Remove(n, v.GroupKind())
			if effClusterResources.Contains(cluster.GetId(), v.GroupKind()) {
				effClusterResources.Remove(cluster.GetId(), v.GroupKind())
				err := v.Deploy(log, cluster, this.env.ControllerManager().GetMaintainer())
				if err != nil {
					return err
				}
			} else {
				log.Infof("crd for %s already handled", v.GroupKind())
			}
		}
		err := this.deployImplicitCustomResourceDefinitions(log, effClusterResources, clusterResources[n], cluster)
		if err != nil {
			return err
		}
		delete(clusterResources, cluster.GetId())
	}
	for n, gks := range clusterResources {
		cluster := this.GetCluster(n)
		if isDeployCRDsDisabled(cluster) || len(gks) == 0 {
			continue
		}
		this.Infof("ensure required crds for cluster %q (used for %q)", cluster.GetName(), n)
		err := this.deployImplicitCustomResourceDefinitions(log, effClusterResources, gks, cluster)
		if err != nil {
			return err
		}
	}
	return nil
}

func isDeployCRDsDisabled(cl cluster.Interface) bool {
	return cl.GetAttr(cluster.SUBOPTION_DISABLE_DEPLOY_CRDS) == true
}

func (this *controller) whenReady() {
	this.ready.WhenReady()
}

func (this *controller) IsReady() bool {
	return this.ready.IsReady()
}

func (this *controller) GetReconciler(name string) reconcile.Interface {
	return this.reconcilers[name]
}

func (this *controller) addReconciler(cname string, spec ReconcilationElementSpec, pool string, reconciler string) (bool, error) {
	r := this.reconcilers[reconciler]
	if r == nil {
		return false, fmt.Errorf("reconciler %q not found for %q", reconciler, spec)
	}

	cluster := this.cluster
	cluster_name := ""
	aliases := utils.StringSet{}
	if cname != "" {
		cluster = this.clusters.GetCluster(cname)
		if cluster == nil {
			return false, fmt.Errorf("cluster %q not found for %q", mappings.ClusterName(cname), spec)
		}
		cluster_name = cluster.GetName()
		aliases = this.clusters.GetAliases(cluster.GetName())
	}
	if gk, ok := spec.(schema.GroupKind); ok {
		if reject, ok := r.(reconcile.ReconcilationRejection); ok {
			if reject.RejectResourceReconcilation(cluster, gk) {
				this.Infof("reconciler %s rejects resource reconcilation resource %s for cluster %s",
					reconciler, gk, cluster.GetName())
				return false, nil
			}
			this.Infof("reconciler %s supports reconcilation rejection and accepts resource %s for cluster %s",
				reconciler, gk, cluster.GetName())
		}
	}

	src := _ReconcilationKey{key: spec, cluster: cluster_name, reconciler: reconciler}
	mapping, ok := this.mappings[src]
	if ok {
		if mapping != pool {
			return false, fmt.Errorf("a key (%s) for the same cluster %q (used for %s) and reconciler (%s) can only be handled by one pool (found %q and %q)", spec, cluster_name, aliases, reconciler, pool, mapping)
		}
	} else {
		this.mappings[src] = pool
	}

	if cname == "" {
		this.Infof("*** adding reconciler %q for %q using pool %q", reconciler, spec, pool)
	} else {
		this.Infof("*** adding reconciler %q for %q in cluster %q (used for %q) using pool %q", reconciler, spec, cluster_name, mappings.ClusterName(cname), pool)
	}
	this.getPool(pool).addReconciler(spec, r)
	return true, nil
}

func (this *controller) getPool(name string) *pool {
	pool := this.pools[name]
	if pool == nil {
		def := this.definition.Pools()[name]

		if def == nil {
			panic(fmt.Sprintf("unknown pool %q for controller %q", name, this.GetName()))
		}
		this.Infof("get pool config %q", def.GetName())
		options := this.options.PrefixedShared().GetSource(def.GetName()).(config.OptionSet)
		size := options.GetOption(POOL_SIZE_OPTION).IntValue()
		period := def.Period()
		if period != 0 {
			period = options.GetOption(POOL_RESYNC_PERIOD_OPTION).DurationValue()
		}
		pool = NewPool(this, name, size, period)
		this.pools[name] = pool
	}
	return pool
}

func (this *controller) GetPool(name string) Pool {
	pool := this.pools[name]
	if pool == nil {
		return nil
	}
	return pool
}

func (this *controller) GetEnvironment() Environment {
	return this.env
}

func (this *controller) GetDefinition() Definition {
	return this.definition
}

func (this *controller) GetOptionSource(name string) (config.OptionSource, error) {
	src := this.options.PrefixedShared().GetSource(CONTROLLER_SET_PREFIX + name)
	if src == nil {
		return nil, fmt.Errorf("option source %s not found for controller %s", name, this.GetName())
	}
	return src, nil
}

func (this *controller) getClusterHandler(name string) (*ClusterHandler, error) {
	cluster := this.GetCluster(name)

	if cluster == nil {
		return nil, fmt.Errorf("unknown cluster %q for %q", name, this.GetName())
	}
	h := this.handlers[cluster.GetId()]
	if h == nil {
		h = newClusterHandler(this, cluster)
		this.handlers[cluster.GetId()] = h
	}
	return h, nil
}

func (this *controller) ClusterHandler(cluster resources.Cluster) *ClusterHandler {
	return this.handlers[cluster.GetId()]
}

func (this *controller) GetClusterById(id string) cluster.Interface {
	return this.clusters.GetById(id)
}

func (this *controller) GetCluster(name string) cluster.Interface {
	if name == CLUSTER_MAIN || name == "" {
		return this.GetMainCluster()
	}
	return this.clusters.GetCluster(name)
}
func (this *controller) GetMainCluster() cluster.Interface {
	return this.cluster
}
func (this *controller) GetClusterAliases(eff string) utils.StringSet {
	return this.clusters.GetAliases(eff)
}
func (this *controller) GetEffectiveCluster(eff string) cluster.Interface {
	return this.clusters.GetEffective(eff)
}

func (this *controller) GetObject(key resources.ClusterObjectKey) (resources.Object, error) {
	return this.clusters.GetObject(key)
}

func (this *controller) GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error) {
	return this.clusters.GetCachedObject(key)
}

func (this *controller) EnqueueKey(key resources.ClusterObjectKey) error {
	cluster := this.GetClusterById(key.Cluster())
	if cluster == nil {
		return fmt.Errorf("cluster with id %q not found", key.Cluster())
	}
	h := this.ClusterHandler(cluster)
	return h.EnqueueKey(key)
}

func (this *controller) Enqueue(object resources.Object) error {
	h := this.ClusterHandler(object.GetCluster())
	return h.EnqueueObject(object)
}

func (this *controller) EnqueueAfter(object resources.Object, duration time.Duration) error {
	h := this.ClusterHandler(object.GetCluster())
	return h.EnqueueObjectAfter(object, duration)
}

func (this *controller) EnqueueRateLimited(object resources.Object) error {
	h := this.ClusterHandler(object.GetCluster())
	return h.EnqueueObjectRateLimited(object)
}

func (this *controller) EnqueueCommand(cmd string) error {
	found := false
	for _, p := range this.pools {
		r := p.getReconcilers(cmd)
		if r != nil && len(r) > 0 {
			p.EnqueueCommand(cmd)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("no handler found for command %q", cmd)
	}
	return nil
}

func (this *controller) Owning() ResourceKey {
	return this.owning.ResourceType()
}

func (this *controller) GetMainWatchResource() WatchResource {
	return this.owning
}

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// controller start up

// Check does all the checks that might cause Prepare to fail
// after a successful check Prepare can execute without error
func (this *controller) check() error {
	h, err := this.getClusterHandler(CLUSTER_MAIN)
	if err != nil {
		return err
	}

	_, err = h.GetResource(this.Owning())
	if err != nil {
		return err
	}

	// setup and check cluster handlers for all required cluster
	for cname, watches := range this.GetDefinition().Watches() {
		h, err := this.getClusterHandler(cname)
		if err != nil {
			return err
		}
		for _, watch := range watches {
			_, err = h.GetResource(watch.ResourceType())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *controller) AddCluster(cluster cluster.Interface) error {
	return nil
}

func (this *controller) registerWatch(h *ClusterHandler, r WatchResource, p string) error {
	var optionsFunc resources.TweakListOptionsFunc
	var ns = ""

	if r.WatchSelectionFunction() != nil {
		ns, optionsFunc = r.WatchSelectionFunction()(this)
	}
	return h.register(r.ResourceType(), ns, optionsFunc, this.getPool(p))
}

// Prepare finally prepares the controller to run
// all error conditions MUST also be checked
// in Check, so after a successful checkController
// startController MUST not return an error.
func (this *controller) prepare() error {
	h, err := this.getClusterHandler(CLUSTER_MAIN)
	if err != nil {
		return err
	}

	this.Infof("setup reconcilers...")
	for n, r := range this.reconcilers {
		err = reconcile.SetupReconciler(r)
		if err != nil {
			return fmt.Errorf("setup of reconciler %s of controller %s failed: %s", n, this.GetName(), err)
		}
	}

	this.Infof("setup watches....")
	this.Infof("watching main resources %q at cluster %q (reconciler %s)", this.Owning(), h, DEFAULT_RECONCILER)

	err = this.registerWatch(h, this.owning, DEFAULT_POOL)
	if err != nil {
		return err
	}
	for cname, watches := range this.watches {
		h, err := this.getClusterHandler(cname)
		if err != nil {
			return err
		}

		for _, watch := range watches {
			this.Infof("watching additional resources %q at cluster %q (reconciler %s)", watch.ResourceType(), h, watch.Reconciler())
			this.registerWatch(h, watch, watch.PoolName())
		}
	}
	this.Infof("setup watches done")

	return nil
}

func (this *controller) Run() {

	this.ready.ready()
	this.Infof("starting pools...")
	for _, p := range this.pools {
		ctxutil.WaitGroupRunAndCancelOnExit(this.GetContext(), p.Run)
	}

	this.Infof("starting reconcilers...")
	for n, r := range this.reconcilers {
		err := reconcile.StartReconciler(r)
		if err != nil {
			this.Errorf("exit controller %s because start of reconciler %s failed: %s", this.GetName(), n, err)
			return
		}
	}
	this.Infof("controller started")
	<-this.GetContext().Done()
	this.Info("waiting for worker pools to shutdown")
	ctxutil.WaitGroupWait(this.GetContext(), 120*time.Second)
	this.Info("exit controller")
}

func (this *controller) mustHandle(r resources.Object) bool {
	for _, f := range this.filters {
		if !f(this.owning.ResourceType(), r) {
			this.Debugf("%s rejected by filter", r.Description())
			return false
		}
	}
	return true
}

func (this *controller) DecodeKey(key string) (string, *resources.ClusterObjectKey, resources.Object, error) {
	i := strings.Index(key, ":")

	if i < 0 {
		return key, nil, nil, nil
	}

	main := key[:i]
	if main == "cmd" {
		return key[i+1:], nil, nil, nil
	}
	if main == "obj" {
		key = key[i+1:]
	}
	i = strings.Index(key, ":")

	cluster := this.clusters.GetEffective(key[0:i])
	if cluster == nil {
		return "", nil, nil, fmt.Errorf("unknown cluster in key %q", key)
	}

	key = key[i+1:]

	apiGroup, kind, namespace, name, err := DecodeObjectSubKey(key)
	if err != nil {
		return "", nil, nil, fmt.Errorf("error decoding '%s': %s", key, err)
	}
	objKey := resources.NewClusterKey(cluster.GetId(), resources.NewGroupKind(apiGroup, kind), namespace, name)

	r, err := this.ClusterHandler(cluster).GetObject(objKey)
	return "", &objKey, r, err
}

func (this *controller) Synchronize(log logger.LogContext, name string, initiator resources.Object) (bool, error) {
	return this.syncRequests.Synchronize(log, name, initiator)
}

func (this *controller) requestHandled(log logger.LogContext, reconciler reconcile.Interface, key resources.ClusterObjectKey) {
	this.syncRequests.requestHandled(log, this.reconcilerNames[reconciler], key)
}
