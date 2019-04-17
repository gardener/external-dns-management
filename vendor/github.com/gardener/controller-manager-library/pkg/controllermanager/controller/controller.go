/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/clientsets/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/client-go/tools/record"
)

type ResourceFilter func(owning ResourceKey, resc resources.Object) bool

type EventRecorder interface {
	// The resulting event will be created in the same namespace as the reference object.
	//Event(object runtime.ObjectData, eventtype, reason, message string)

	// Eventf is just like Event, but with Sprintf for the message field.
	//Eventf(object runtime.ObjectData, eventtype, reason, messageFmt string, args ...interface{})

	// PastEventf is just like Eventf, but with an option to specify the event's 'timestamp' field.
	//PastEventf(object runtime.ObjectData, timestamp metav1.Time, eventtype, reason, messageFmt string, args ...interface{})

	// AnnotatedEventf is just like eventf, but with annotations attached
	//AnnotatedEventf(object runtime.ObjectData, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})
}

type Environment interface {
	GetContext() context.Context
	GetClusters() cluster.Clusters
	GetCluster(name string) cluster.Interface
	GetConfig() *config.Config
	GetSharedValue(key interface{}) interface{}
	//GetSharedOption(name string) *config.ArbitraryOption
}

type _ReconcilerMapping struct {
	key        interface{}
	cluster    string
	reconciler string
}

type SharedAttributes struct {
	logger.LogContext
	lock   sync.RWMutex
	shared map[interface{}]interface{}
}

func (c *SharedAttributes) GetSharedValue(key interface{}) interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.shared == nil {
		return nil
	}
	return c.shared[key]
}

func (c *SharedAttributes) GetOrCreateSharedValue(key interface{}, create func() interface{}) interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.shared == nil {
		c.shared = map[interface{}]interface{}{}
	}
	v, ok := c.shared[key]
	if !ok {
		c.Infof("creating shared value for key %v", key)
		v = create()
		c.shared[key] = v
	}
	return v
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
	record.EventRecorder
	SharedAttributes

	ready       ReadyFlag
	definition  Definition
	env         Environment
	ctx         context.Context
	cluster     cluster.Interface
	clusters    cluster.Clusters
	filters     []ResourceFilter
	owning      ResourceKey
	reconcilers map[string]reconcile.Interface
	mappings    map[_ReconcilerMapping]string
	finalizer   Finalizer

	handlers map[string]*ClusterHandler

	pools map[string]*pool
}

func Filter(owning ResourceKey, resc resources.Object) bool {
	return true
}

func NewController(env Environment, def Definition, cmp mappings.Definition) (*controller, error) {

	required := cluster.Canonical(def.RequiredClusters())
	clusters, err := mappings.MapClusters(env.GetClusters(), cmp, required...)
	if err != nil {
		return nil, err
	}
	cluster := clusters.GetCluster(required[0])

	this := &controller{
		EventRecorder: cluster.Resources(),

		definition: def,
		env:        env,
		cluster:    cluster,
		clusters:   clusters,

		owning:  def.MainResource(),
		filters: def.ResourceFilters(),

		handlers:    map[string]*ClusterHandler{},
		pools:       map[string]*pool{},
		reconcilers: map[string]reconcile.Interface{},
		mappings:    map[_ReconcilerMapping]string{},
		finalizer:   NewDefaultFinalizer(def.FinalizerName()),
	}

	this.ready.start()

	this.ctx, this.LogContext = logger.WithLogger(
		ctxutil.SyncContext(
			context.WithValue(env.GetContext(), typekey, this)),
		"this", def.GetName())
	this.Infof("  using clusters %+v: %s (selected from %s)", required, clusters, env.GetClusters())

	for n, crds := range def.CustomResourceDefinitions() {
		cluster := clusters.GetCluster(n)
		if cluster == nil {
			return nil, fmt.Errorf("cluster %q not found for resource definitions", n)
		}
		this.Infof("create required crds for cluster %q (used for %q)", cluster.GetName(), n)
		for _, crd := range crds {
			this.Infof("   %s", crd.Name)
			apiextensions.CreateCRDFromObject(cluster.Clientsets(), crd)
		}
	}
	for n, t := range def.Reconcilers() {
		this.Infof("creating reconciler %q", n)
		reconciler, err := t(this)
		if err != nil {
			return nil, err
		}
		this.reconcilers[n] = reconciler
	}

	for cname, watches := range this.definition.Watches() {
		for _, w := range watches {
			err := this.addReconciler(cname, w.ResourceType().GroupKind(), w.PoolName(), w.Reconciler())
			if err != nil {
				this.Errorf("GOT error: %s", err)
				return nil, err
			}
		}
	}
	err = this.addReconciler(required[0], this.Owning().GroupKind(), DEFAULT_POOL, DEFAULT_RECONCILER)
	if err != nil {
		return nil, err
	}

	for _, cmds := range this.GetDefinition().Commands() {
		for _, cmd := range cmds {
			err := this.addReconciler("", cmd.Key(), cmd.PoolName(), cmd.Reconciler())
			if err != nil {
				return nil, err
			}
		}
	}

	return this, nil
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

func (this *controller) addReconciler(cname string, key interface{}, pool string, reconciler string) error {
	r := this.reconcilers[reconciler]
	if r == nil {
		return fmt.Errorf("reconciler %q not found for %q", reconciler, key)
	}
	cluster_name := ""
	aliases := utils.StringSet{}
	if cname != "" {
		cluster := this.clusters.GetCluster(cname)
		if cluster == nil {
			return fmt.Errorf("cluster %q not found for %q", mappings.ClusterName(cname), key)
		}
		cluster_name = cluster.GetName()
		aliases = this.clusters.GetAliases(cluster.GetName())
	}
	src := _ReconcilerMapping{key: key, cluster: cluster_name, reconciler: reconciler}
	mapping, ok := this.mappings[src]
	if ok {
		if mapping != pool {
			return fmt.Errorf("a key (%s) for the same cluster %q (used for %s) and reconciler (%s) can only be handled by one pool (found %q and %q)", key, cluster_name, aliases, reconciler, pool, mapping)
		}
	} else {
		this.mappings[src] = pool
	}

	if cname == "" {
		this.Infof("*** adding reconciler %q for %q using pool %q", reconciler, key, pool)
	} else {
		this.Infof("*** adding reconciler %q for %q in cluster %q (used for %q) using pool %q", reconciler, key, cluster_name, mappings.ClusterName(cname), pool)
	}
	this.getPool(pool).addReconciler(key, r)
	return nil
}

func (this *controller) getPool(name string) *pool {
	pool := this.pools[name]
	if pool == nil {
		def := this.definition.Pools()[name]
		if def == nil {
			def = &pooldef{name: name, size: 5, period: 30 * time.Second}
		}
		size := def.Size()
		{
			opt := this.env.GetConfig().GetOption(PoolSizeOptionName(this.GetName(), name))

			if shared := this.env.GetConfig().GetOption(POOL_SIZE_OPTION); shared != nil && shared.Changed() && (opt == nil || !opt.Changed()) {
				if shared != nil && shared.Changed() {
					opt = shared
				}
			}
			if opt != nil {
				size = opt.IntValue()
			}
		}

		period := def.Period()
		{
			opt := this.env.GetConfig().GetOption(PoolResyncPeriodOptionName(this.GetName(), name))

			if shared := this.env.GetConfig().GetOption(POOL_RESYNC_PERIOD_OPTION); shared != nil && shared.Changed() && (opt == nil || !opt.Changed()) {
				if shared != nil && shared.Changed() {
					opt = shared
				}
			}
			if opt != nil {
				period = opt.DurationValue()
			}
		}

		pool = NewPool(this, name, size, period)
		this.pools[name] = pool
	}
	return pool
}

func (this *controller) GetPool(name string) Pool {
	return this.pools[name]
}

func (this *controller) GetName() string {
	return this.definition.GetName()
}

func (this *controller) GetEnvironment() Environment {
	return this.env
}

func (this *controller) GetDefinition() Definition {
	return this.definition
}

func (this *controller) GetClusterHandler(name string) (*ClusterHandler, error) {
	cluster := this.GetCluster(name)

	if cluster == nil {
		return nil, fmt.Errorf("unknown cluster %q for %q", name, this.GetName())
	}
	h := this.handlers[cluster.GetName()]
	if h == nil {
		h = newClusterHandler(this, cluster)
		this.handlers[cluster.GetName()] = h
	}
	return h, nil
}

func (this *controller) GetClusterById(id string) cluster.Interface {
	return this.clusters.GetById(id)
}

func (this *controller) GetCluster(name string) cluster.Interface {
	if name == CLUSTER_MAIN {
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
	h := this.handlers[cluster.GetName()]
	return h.EnqueueKey(key)
}

func (this *controller) Enqueue(object resources.Object) error {
	h := this.handlers[object.GetCluster().GetName()]
	return h.EnqueueObject(object)
}

func (this *controller) EnqueueAfter(object resources.Object, duration time.Duration) error {
	h := this.handlers[object.GetCluster().GetName()]
	return h.EnqueueObjectAfter(object, duration)
}

func (this *controller) EnqueueRateLimited(object resources.Object) error {
	h := this.handlers[object.GetCluster().GetName()]
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
	return this.owning
}

func (this *controller) GetContext() context.Context {
	return this.ctx
}

func (this *controller) GetOption(name string) (*config.ArbitraryOption, error) {
	n := ControllerOption(this.GetName(), name)
	opt := this.env.GetConfig().GetOption(n)
	if opt == nil {
		return nil, fmt.Errorf("unknown option %q for controller %q", name, this.GetName())
	}
	shared := this.env.GetConfig().GetOption(name)
	/*
		this.Infof("getting option %q(%q) for controller %q [changed %t]", name, n, this.GetName(), opt.Changed())
		if shared != nil {
			this.Infof("   shared option %q [changed %t]", shared.Name, shared.Changed())
		}
	*/
	if !opt.Changed() && shared != nil && shared.Changed() {
		opt = shared
	}
	return opt, nil
}

func (this *controller) GetBoolOption(name string) (bool, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return false, err
	}
	return opt.BoolValue(), nil
}
func (this *controller) GetStringOption(name string) (string, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return "", err
	}
	return opt.StringValue(), nil
}
func (this *controller) GetStringArrayOption(name string) ([]string, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return []string{}, err
	}
	return opt.StringArray(), nil
}
func (this *controller) GetIntOption(name string) (int, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return 0, err
	}
	return opt.IntValue(), nil
}
func (this *controller) GetDurationOption(name string) (time.Duration, error) {
	opt, err := this.GetOption(name)
	if err != nil {
		return 0, err
	}
	return opt.DurationValue(), nil
}

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// controller start up

// Check does all the checks that might cause Prepare to fail
// after a successful check Prepare can execute without error
func (this *controller) Check() error {
	h, err := this.GetClusterHandler(CLUSTER_MAIN)
	if err != nil {
		return err
	}

	_, err = h.GetResource(this.Owning())
	if err != nil {
		return err
	}

	// setup and check cluster handlers for all required cluster
	for cname, watches := range this.GetDefinition().Watches() {
		_, err := this.GetClusterHandler(cname)
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

// Prepare finally prepares the controller to run
// all error conditions MUST also be checked
// in Check, so after a successful checkController
// startController MUST not return an error.
func (this *controller) Prepare() error {
	h, err := this.GetClusterHandler(CLUSTER_MAIN)
	if err != nil {
		return err
	}

	this.Infof("setup reconcilers...")
	for _, r := range this.reconcilers {
		r.Setup()
	}

	this.Infof("setup watches....")
	this.Infof("watching main resources %q at cluster %q", this.Owning(), h)

	err = h.register(this.Owning(), this.getPool(DEFAULT_POOL))
	if err != nil {
		return err
	}
	for cname, watches := range this.GetDefinition().Watches() {
		h, err := this.GetClusterHandler(cname)
		if err != nil {
			return err
		}

		for _, watch := range watches {
			this.Infof("watching additional resources %q at cluster %q", watch.ResourceType(), h)
			p := this.getPool(watch.PoolName())
			h.register(watch.ResourceType(), p)
		}
	}
	this.Infof("setup watches done")

	return nil
}

func (this *controller) Run() {

	this.ready.ready()
	this.Infof("starting pools...")
	for _, p := range this.pools {
		ctxutil.SyncPointRunAndCancelOnExit(this.ctx, p.Run)
	}

	this.Infof("starting reconcilers...")
	for _, r := range this.reconcilers {
		r.Start()
	}
	this.Infof("controller started")
	<-this.ctx.Done()
	this.Info("waiting for worker pools to shutdown")
	ctxutil.SyncPointWait(this.ctx, 120*time.Second)
	this.Info("exit controller")
}

func (this *controller) mustHandle(r resources.Object) bool {
	for _, f := range this.filters {
		if !f(this.owning, r) {
			this.Infof("%s rejected by filter %v", r.Description(), f)
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

	r, err := cluster.GetCachedObject(objKey)
	return "", &objKey, r, err
}
