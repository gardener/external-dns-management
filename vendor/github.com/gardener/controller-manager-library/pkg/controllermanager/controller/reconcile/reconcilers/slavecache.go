/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcilers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type Resources func(c controller.Interface) []resources.Interface

////////////////////////////////////////////////////////////////////////////////
// SlaveAccess to be used as common nested base for all reconcilers
// requiring slave access
////////////////////////////////////////////////////////////////////////////////

type SlaveAccessSink interface {
	InjectSlaveAccess(*SlaveAccess)
}

type ClusterGroupKinds map[string]resources.GroupKindSet

func (this ClusterGroupKinds) Add(r resources.Interface) ClusterGroupKinds {
	set := this[r.GetCluster().GetId()]
	if set == nil {
		set = resources.GroupKindSet{}
		this[r.GetCluster().GetId()] = set
	}
	set.Add(r.GroupKind())
	return this
}

func (this ClusterGroupKinds) Contains(clusterid string, gk schema.GroupKind) bool {
	return this[clusterid].Contains(gk)
}

func (this ClusterGroupKinds) Matches(k resources.ClusterObjectKey) bool {
	return this[k.Cluster()].Contains(k.GroupKind())
}

type _resources struct {
	kinds     []schema.GroupKind
	resources []resources.Interface
	clusters  ClusterGroupKinds
}

func (this *_resources) Contains(clusterid string, g schema.GroupKind) bool {
	return this.clusters[clusterid].Contains(g)
}

func (this *_resources) String() string {
	str := ""
	if this != nil {
		for _, e := range this.resources {
			str += "|" + e.GroupKind().String()
		}
		str += "|"
	}
	return str
}

func newResources(c controller.Interface, f Resources) *_resources {
	kinds := resources.GroupKindSet{}
	clusters := map[string]resources.GroupKindSet{}
	res := f(c)
	for _, r := range res {
		set := clusters[r.GetCluster().GetId()]
		if set == nil {
			set = resources.GroupKindSet{}
			clusters[r.GetCluster().GetId()] = set
		}
		kinds.Add(r.GroupKind())
		set.Add(r.GroupKind())
	}
	return &_resources{
		kinds:     kinds.AsArray(),
		resources: res,
		clusters:  clusters,
	}
}

////////////////////////////////////////////////////////////////////////////////
// SlaveAccess used to access a shared slave cache
////////////////////////////////////////////////////////////////////////////////

type SlaveAccess struct {
	controller.Interface
	reconcile.DefaultReconciler
	name             string
	slaves           *resources.SlaveCache
	slave_resources  *_resources
	master_resources *_resources
	slavefilters     []resources.ObjectFilter
	migration        resources.ClusterIdMigration
	gkMigration      resources.GroupKindMigration
	spec             SlaveAccessSpec
}

type SlaveAccessSpec struct {
	Name            string
	Namespace       string
	Slaves          Resources
	Masters         Resources
	RequeueDeleting bool

	ClusterIdMigration resources.ClusterIdMigration
	GroupKindMigration resources.GroupKindMigration
}

func NewSlaveAccessSpec(c controller.Interface, name string, slave_func Resources, master_func Resources) SlaveAccessSpec {
	return SlaveAccessSpec{
		Name:               name,
		Slaves:             slave_func,
		Masters:            master_func,
		ClusterIdMigration: c.GetEnvironment().ControllerManager().GetClusterIdMigration(),
		GroupKindMigration: c.GetEnvironment().ControllerManager().GetGroupKindMigration(),
	}
}

func NewSlaveAccess(c controller.Interface, name string, slave_func Resources, master_func Resources) *SlaveAccess {
	return NewSlaveAccessBySpec(c, NewSlaveAccessSpec(c, name, slave_func, master_func))
}

func NewSlaveAccessBySpec(c controller.Interface, spec SlaveAccessSpec) *SlaveAccess {
	return &SlaveAccess{
		Interface:        c,
		name:             spec.Name,
		slave_resources:  newResources(c, spec.Slaves),
		master_resources: newResources(c, spec.Masters),
		migration:        spec.ClusterIdMigration,
		gkMigration:      spec.GroupKindMigration,
		spec:             spec,
	}
}

type accesskey struct {
	name      string
	namespace string
	masters   string
	slaves    string
}

func (this accesskey) String() string {
	return fmt.Sprintf("%s/%s:[%s%s]", this.namespace, this.name, this.masters, this.slaves)
}

func (this *SlaveAccess) Key() interface{} {
	return accesskey{name: this.name, namespace: this.spec.Namespace, masters: this.master_resources.String(), slaves: this.slave_resources.String()}
}

func (this *SlaveAccess) Setup() {
	this.slaves = this.GetOrCreateSharedValue(this.Key(), this.setupSlaveCache).(*resources.SlaveCache)
}

func (this *SlaveAccess) AddSlaveFilter(filter ...resources.ObjectFilter) {
	if this.slaves != nil {
		this.slaves.AddSlaveFilter(filter...)
	}
	this.slavefilters = append(this.slavefilters, filter...)
}

func (this *SlaveAccess) setupSlaveCache() interface{} {
	cache := resources.NewSlaveCache(this.migration, this.gkMigration)
	cache.AddSlaveFilter(this.slavefilters...)
	this.Infof("setup %s owner cache", this.name)
	for _, r := range this.slave_resources.resources {
		list, _ := listCachedWithNamespace(r, this.spec.Namespace)
		cache.Setup(list)
	}
	this.Infof("found %d %s(s) for %d owners", cache.SlaveCount(), this.name, cache.Size())
	return cache
}

func (this *SlaveAccess) SlaveResoures() []resources.Interface {
	return this.slave_resources.resources
}

func (this *SlaveAccess) MasterResoures() []resources.Interface {
	return this.master_resources.resources
}

func (this *SlaveAccess) CreateSlave(obj resources.Object, slave resources.Object) error {
	return this.slaves.CreateSlave(obj, slave)
}

func (this *SlaveAccess) CreateOrModifySlave(obj resources.Object, slave resources.Object, mod resources.Modifier) (bool, error) {
	return this.slaves.CreateOrModifySlave(obj, slave, mod)
}

func (this *SlaveAccess) UpdateSlave(slave resources.Object) error {
	return this.slaves.UpdateSlave(slave)
}

func (this *SlaveAccess) AddSlave(obj resources.Object, slave resources.Object) error {
	return this.slaves.AddSlave(obj, slave)
}

func (this *SlaveAccess) LookupSlaves(key resources.ClusterObjectKey, kinds ...schema.GroupKind) []resources.Object {
	found := []resources.Object{}

	for _, o := range this.slaves.GetByOwnerKey(key) {
		if len(kinds) == 0 {
			if this.slave_resources.clusters[o.GetCluster().GetId()].Contains(o.GroupKind()) {
				found = append(found, o)
			}
		} else {
			for _, k := range kinds {
				if o.GroupKind() == k {
					found = append(found, o)
				}
			}
		}
	}
	return found
}

func (this *SlaveAccess) AssertSingleSlave(logger logger.LogContext, key resources.ClusterObjectKey, slaves []resources.Object, match resources.ObjectMatcher) resources.Object {
	var found resources.Object
	for _, o := range slaves {
		if match == nil || match(o) {
			if found != nil {
				if o.GetCreationTimestamp().Time.Before(found.GetCreationTimestamp().Time) {
				} else {
					found, o = o, found
				}
				err := o.Delete()
				if err != nil {
					logger.Warnf("cleanup of obsolete %s %s for %s failed %s", this.name, o.ObjectName(), key.ObjectName(), err)
				} else {
					logger.Infof("cleanup of obsolete %s %s for %s", this.name, o.ObjectName(), key.ObjectName())
				}
			} else {
				found = o
			}
		}
	}
	return found
}

func (this *SlaveAccess) Slaves() *resources.SlaveCache {
	return this.slaves
}

func (this *SlaveAccess) GetMastersFor(key resources.ClusterObjectKey, all_clusters bool, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	var set resources.ClusterObjectKeySet
	if len(kinds) > 0 {
		set = this.slaves.GetOwnersFor(key, kinds...)
	} else {
		set = this.slaves.GetOwnersFor(key, this.master_resources.kinds...)
	}
	if all_clusters {
		return set
	}
	return filterKeysByClusters(set, this.master_resources.clusters)
}

func (this *SlaveAccess) GetMasters(all_clusters bool, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	var set resources.ClusterObjectKeySet
	if len(kinds) > 0 {
		set = this.slaves.GetOwners(kinds...)
	} else {
		set = this.slaves.GetOwners(this.master_resources.kinds...)
	}
	if all_clusters {
		return set
	}
	return filterKeysByClusters(set, this.master_resources.clusters)
}

func filterKeysByClusters(set resources.ClusterObjectKeySet, clusters ClusterGroupKinds, kinds ...schema.GroupKind) resources.ClusterObjectKeySet {
	if clusters == nil {
		return set
	}
	new := resources.ClusterObjectKeySet{}
	for k := range set {
		if clusters.Matches(k) {
			new.Add(k)
		}
	}
	if len(kinds) != 0 {
	outer:
		for k := range new {
			for _, g := range kinds {
				if k.GroupKind() == g {
					continue outer
				}
			}
			new.Remove(k)
		}
	}
	return new
}

////////////////////////////////////////////////////////////////////////////////
// SlaveReconciler used as Reconciler registered for watching slave object
//  nested reconcilers can cast the controller interface to *SlaveReconciler
////////////////////////////////////////////////////////////////////////////////

func SlaveReconcilerType(name string, slaveResources Resources, reconciler controller.ReconcilerType, masterResources Resources) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return NewSlaveReconciler(c, name, slaveResources, reconciler, masterResources)
	}
}

type SlaveAccessSpecCreator func(c controller.Interface) SlaveAccessSpec

func SlaveReconcilerTypeByFunction(reconciler controller.ReconcilerType, creator SlaveAccessSpecCreator) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return NewSlaveReconcilerBySpec(c, reconciler, creator(c))
	}
}

func SlaveReconcilerTypeBySpec(reconciler controller.ReconcilerType, spec SlaveAccessSpec) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return NewSlaveReconcilerBySpec(c, reconciler, spec)
	}
}

func NewSlaveReconciler(c controller.Interface, name string, slaveResources Resources, reconciler controller.ReconcilerType, masterResources Resources) (*SlaveReconciler, error) {
	return NewSlaveReconcilerBySpec(c, reconciler, SlaveAccessSpec{Name: name, Slaves: slaveResources, Masters: masterResources})
}

func NewSlaveReconcilerBySpec(c controller.Interface, reconciler controller.ReconcilerType, spec SlaveAccessSpec) (*SlaveReconciler, error) {
	r := &SlaveReconciler{
		SlaveAccess: NewSlaveAccessBySpec(c, spec),
	}
	nested, err := NewNestedReconciler(reconciler, r)
	if err != nil {
		return nil, err
	}
	if s, ok := nested.nested.(SlaveAccessSink); ok {
		s.InjectSlaveAccess(r.SlaveAccess)
	}
	r.NestedReconciler = nested
	return r, nil
}

type SlaveReconciler struct {
	*NestedReconciler
	*SlaveAccess
}

var _ reconcile.Interface = &SlaveReconciler{}

func (this *SlaveReconciler) Setup() error {
	this.SlaveAccess.Setup()
	return this.NestedReconciler.Setup()
}

func (this *SlaveReconciler) Start() error {
	this.Infof("determining dangling %s objects...", this.spec.Name)
	for k := range this.SlaveAccess.GetMasters(false) {
		if this.master_resources.Contains(k.Cluster(), k.GroupKind()) {
			if _, err := this.GetClusterById(k.Cluster()).GetCachedObject(k); errors.IsNotFound(err) {
				this.Infof("trigger vanished origin %s", k.ObjectKey())
				this.EnqueueKey(k)
			} else {
				this.Debugf("found origin %s", k.ObjectKey())
			}
		}
	}
	return this.NestedReconciler.Start()
}

func (this *SlaveReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.slaves.RenewSlaveObject(obj)
	logger.Infof("reconcile slave %s", obj.ClusterKey())
	this.requeueMasters(logger, this.GetMastersFor(obj.ClusterKey(), false))
	return this.NestedReconciler.Reconcile(logger, obj)
}

func (this *SlaveReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	masters := this.GetMastersFor(key, false)
	this.slaves.DeleteSlave(key)
	this.requeueMasters(logger, masters)
	return this.NestedReconciler.Deleted(logger, key)
}

func (this *SlaveReconciler) requeueMasters(logger logger.LogContext, masters resources.ClusterObjectKeySet) {
	for key := range masters {
		m, err := this.GetObject(key)
		if err == nil || errors.IsNotFound(err) {
			if !this.spec.RequeueDeleting && m.IsDeleting() {
				logger.Infof("skipping requeue of deleting master %s", key)
				continue
			}
		}
		logger.Infof("requeue master %s", key)
		this.EnqueueKey(key)
	}
}
