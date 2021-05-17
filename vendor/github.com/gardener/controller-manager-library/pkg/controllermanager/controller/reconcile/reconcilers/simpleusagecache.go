/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcilers

import (
	"fmt"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/filter"
)

var usersKey = ctxutil.SimpleKey("users")

// GetSharedSimpleUsageCache returns an instance of a usage cache unique for
// the complete controller manager
func GetSharedSimpleUsageCache(controller controller.Interface) *SimpleUsageCache {
	return controller.GetEnvironment().GetOrCreateSharedValue(usersKey, func() interface{} {
		return NewSimpleUsageCache()
	}).(*SimpleUsageCache)
}

type RelationKey struct {
	From resources.ClusterGroupKind
	To   resources.ClusterGroupKind
}

type SimpleUsageCache struct {
	resources.ClusterObjectKeyLocks
	lock        sync.RWMutex
	users       map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	uses        map[resources.ClusterObjectKey]resources.ClusterObjectKeySet
	reconcilers controller.WatchedResources
	initialized map[RelationKey]bool
}

func NewSimpleUsageCache() *SimpleUsageCache {
	return &SimpleUsageCache{
		users:       map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		uses:        map[resources.ClusterObjectKey]resources.ClusterObjectKeySet{},
		reconcilers: controller.WatchedResources{},
		initialized: map[RelationKey]bool{},
	}
}

// reconcilerFor is used to assure that only one reconciler in one controller
// handles the usage reconcilations. The usage cache is hold at controller
// extension level and is shared among all controllers of a controller manager.
func (this *SimpleUsageCache) reconcilerFor(set resources.ClusterGroupKindSet) resources.ClusterGroupKindSet {
	responsible := resources.ClusterGroupKindSet{}

	this.lock.Lock()
	defer this.lock.Unlock()
	for cgk := range set {
		if !this.reconcilers.Contains(cgk.Cluster, cgk.GroupKind) {
			responsible.Add(cgk)
			this.reconcilers.GatheredAdd(cgk.Cluster, nil, cgk.GroupKind)
		}
	}
	return responsible
}

func (this *SimpleUsageCache) GetUsersFor(name resources.ClusterObjectKey) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.users[name]
	if set == nil {
		return nil
	}
	return set.Copy()
}

func (this *SimpleUsageCache) GetFilteredUsersFor(name resources.ClusterObjectKey, filter resources.KeyFilter) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.users[name]
	if set == nil {
		return nil
	}
	copy := resources.NewClusterObjectKeySet()
	for k := range set {
		if filter == nil || filter(k) {
			copy.Add(k)
		}
	}
	return copy
}

func (this *SimpleUsageCache) GetUsesFor(name resources.ClusterObjectKey) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.uses[name]
	if set == nil {
		return nil
	}
	return set.Copy()
}

func (this *SimpleUsageCache) GetFilteredUsesFor(name resources.ClusterObjectKey, filter resources.KeyFilter) resources.ClusterObjectKeySet {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.uses[name]
	if set == nil {
		return nil
	}
	copy := resources.NewClusterObjectKeySet()
	for k := range set {
		if filter == nil || filter(k) {
			copy.Add(k)
		}
	}
	return copy
}

func (this *SimpleUsageCache) SetUsesFor(user resources.ClusterObjectKey, used *resources.ClusterObjectKey) {
	if used == nil {
		this.UpdateUsesFor(user, nil)
	} else {
		this.UpdateUsesFor(user, resources.NewClusterObjectKeySet(*used))
	}
}

func (this *SimpleUsageCache) UpdateUsesFor(user resources.ClusterObjectKey, uses resources.ClusterObjectKeySet) {
	this.UpdateFilteredUsesFor(user, nil, uses)
}

func (this *SimpleUsageCache) UpdateFilteredUsesFor(user resources.ClusterObjectKey, filter resources.KeyFilter, uses resources.ClusterObjectKeySet) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.updateFilteredUsesFor(user, filter, uses)
}

func (this *SimpleUsageCache) updateFilteredUsesFor(user resources.ClusterObjectKey, filter resources.KeyFilter, uses resources.ClusterObjectKeySet) {
	uses = uses.Filter(filter)
	var add, del resources.ClusterObjectKeySet
	old := this.uses[user].Filter(filter)
	if old != nil {
		add, del = old.DiffFrom(uses)
		this.cleanup(user, del)
	} else {
		add = uses
	}
	if len(add) > 0 {
		for used := range uses {
			this.addUseFor(user, used)
		}
	}
}

func (this *SimpleUsageCache) AddUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.addUseFor(user, used)
}

func (this *SimpleUsageCache) addUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	set := this.users[used]
	if set == nil {
		set = resources.NewClusterObjectKeySet()
		this.users[used] = set
	}
	set.Add(user)

	set = this.uses[user]
	if set == nil {
		set = resources.NewClusterObjectKeySet()
		this.uses[user] = set
	}
	set.Add(used)
}

func (this *SimpleUsageCache) RemoveUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.removeUseFor(user, used)
}

func (this *SimpleUsageCache) removeUseFor(user resources.ClusterObjectKey, used resources.ClusterObjectKey) {
	set := this.users[used]
	if set != nil {
		set.Remove(user)
		if len(set) == 0 {
			delete(this.users, used)
		}
	}

	set = this.uses[user]
	if set != nil {
		set.Remove(used)
		if len(set) == 0 {
			delete(this.uses, user)
		}
	}
}

func (this *SimpleUsageCache) cleanup(user resources.ClusterObjectKey, uses resources.ClusterObjectKeySet) {
	if len(uses) > 0 {
		for l := range uses {
			this.removeUseFor(user, l)
		}
	}
}

func (this *SimpleUsageCache) SetupFor(log logger.LogContext, resc resources.Interface, extract resources.UsedExtractor) error {
	return ProcessResource(log, "setup", resc, func(log logger.LogContext, obj resources.Object) (bool, error) {
		used := extract(obj)
		this.UpdateUsesFor(obj.ClusterKey(), used)
		return true, nil
	})
}

func (this *SimpleUsageCache) SetupFilteredFor(log logger.LogContext, resc resources.Interface, filter resources.KeyFilter, extract resources.UsedExtractor) error {
	return ProcessResource(log, "setup", resc, func(log logger.LogContext, obj resources.Object) (bool, error) {
		used := extract(obj)
		this.UpdateFilteredUsesFor(obj.ClusterKey(), filter, used)
		return true, nil
	})
}

func (this *SimpleUsageCache) setupFilteredFor(log logger.LogContext, resc resources.Interface, filter resources.KeyFilter, extract resources.UsedExtractor) error {
	return ProcessResource(log, "setup", resc, func(log logger.LogContext, obj resources.Object) (bool, error) {
		used := extract(obj)
		this.updateFilteredUsesFor(obj.ClusterKey(), filter, used)
		return true, nil
	})
}

func (this *SimpleUsageCache) SetupForRelation(log logger.LogContext, c cluster.Interface, from schema.GroupKind,
	to resources.ClusterGroupKind, extractor resources.UsedExtractor) error {
	key := RelationKey{
		From: resources.NewClusterGroupKind(c.GetId(), from),
		To:   to,
	}
	this.lock.Lock()
	defer this.lock.Unlock()
	if !this.initialized[key] {
		log.Infof("setting up usages of %q for %q...", key.From, key.To)
		this.initialized[key] = true
		res, err := c.GetResource(from)
		if err == nil {
			err = this.setupFilteredFor(log, res, filter.ClusterGroupKindFilter(to), extractor)
		}
		return err
	} else {
		log.Infof("setup up usages of %q for %q already done", key.From, key.To)
	}
	return nil
}

func (this *SimpleUsageCache) CleanupUser(log logger.LogContext, msg string, controller controller.Interface, user resources.ClusterObjectKey, actions ...KeyAction) error {
	used := this.GetUsesFor(user)
	if len(actions) > 0 {
		if len(used) > 0 && log != nil && msg != "" {
			log.Infof("%s %d uses of %s", msg, len(used), user.ObjectKey())
		}
		if err := this.execute(log, controller, used, actions...); err != nil {
			return err
		}
	}
	this.UpdateUsesFor(user, nil)
	return nil
}

func (this *SimpleUsageCache) ExecuteActionForUsersOf(log logger.LogContext, msg string, controller controller.Interface, used resources.ClusterObjectKey, actions ...KeyAction) error {
	return this.ExecuteActionForFilteredUsersOf(log, msg, controller, used, nil, actions...)
}

func (this *SimpleUsageCache) ExecuteActionForFilteredUsersOf(log logger.LogContext, msg string, controller controller.Interface, key resources.ClusterObjectKey, filter resources.KeyFilter, actions ...KeyAction) error {
	if len(actions) > 0 {
		users := this.GetFilteredUsersFor(key, filter)
		if len(users) > 0 && log != nil && msg != "" {
			log.Infof("%s %d users of %s", msg, len(users), key.ObjectKey())
		}
		return this.execute(log, controller, users, actions...)
	}
	return nil
}

func (this *SimpleUsageCache) ExecuteActionForUsesOf(log logger.LogContext, msg string, controller controller.Interface, key resources.ClusterObjectKey, actions ...KeyAction) error {
	return this.ExecuteActionForFilteredUsesOf(log, msg, controller, key, nil, actions...)
}

func (this *SimpleUsageCache) ExecuteActionForFilteredUsesOf(log logger.LogContext, msg string, controller controller.Interface, key resources.ClusterObjectKey, filter resources.KeyFilter, actions ...KeyAction) error {
	if len(actions) > 0 {
		used := this.GetFilteredUsesFor(key, filter)
		if len(used) > 0 && log != nil && msg != "" {
			log.Infof("%s %d uses of %s", msg, len(used), key.ObjectKey())
		}
		return this.execute(log, controller, used, actions...)
	}
	return nil
}

func (this *SimpleUsageCache) execute(log logger.LogContext, controller controller.Interface, keys resources.ClusterObjectKeySet, actions ...KeyAction) error {
	for key := range keys {
		for _, a := range actions {
			err := a(log, controller, key)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// LockAndUpdateFilteredUsage updates the usage of an object of a dedicated kind for a single used object
// the used object is locked and an unlock function returned
func (this *SimpleUsageCache) LockAndUpdateFilteredUsage(user resources.ClusterObjectKey, filter resources.KeyFilter, used resources.ClusterObjectKey) func() {
	this.Lock(nil, used)
	this.UpdateFilteredUsesFor(user, filter, resources.NewClusterObjectKeySet(used))
	return func() { this.Unlock(used) }
}

// LockAndUpdateFilteredUsages updates the usage of an object of a dedicated kind
// the used object is locked and an unlock function returned
func (this *SimpleUsageCache) LockAndUpdateFilteredUsages(user resources.ClusterObjectKey, filter resources.KeyFilter, used resources.ClusterObjectKeySet) func() {
	keys := used.AsArray()
	sort.Sort(keys)
	for _, key := range keys {
		this.Lock(nil, key)
	}
	this.UpdateFilteredUsesFor(user, filter, used)
	return func() {
		for _, key := range keys {
			this.Unlock(key)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

type KeyAction func(log logger.LogContext, c controller.Interface, key resources.ClusterObjectKey) error

func EnqueueAction(log logger.LogContext, c controller.Interface, key resources.ClusterObjectKey) error {
	return c.EnqueueKey(key)
}

func GlobalEnqueueAction(log logger.LogContext, c controller.Interface, key resources.ClusterObjectKey) error {
	c.GetEnvironment().EnqueueKey(key)
	return nil
}

type ObjectAction func(log logger.LogContext, controller controller.Interface, obj resources.Object) error

func ObjectAsKeyAction(actions ...ObjectAction) KeyAction {
	return func(log logger.LogContext, controller controller.Interface, key resources.ClusterObjectKey) error {
		if len(actions) > 0 {
			obj, err := controller.GetClusterById(key.Cluster()).Resources().GetCachedObject(key.ObjectKey())
			if err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
			} else {
				for _, a := range actions {
					err := a(log, controller, obj)
					if err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
}

func RemoveFinalizerObjectAction(log logger.LogContext, controller controller.Interface, obj resources.Object) error {
	return controller.RemoveFinalizer(obj)
}

func RemoveFinalizerAction() KeyAction {
	return ObjectAsKeyAction(RemoveFinalizerObjectAction)
}

////////////////////////////////////////////////////////////////////////////////

type usageReconciler struct {
	ReconcilerSupport
	cache       *SimpleUsageCache
	responsible resources.ClusterGroupKindSet
	relations   *ControllerUsageRelations
}

var _ reconcile.Interface = &usageReconciler{}
var _ reconcile.ReconcilationRejection = &usageReconciler{}

func (this *usageReconciler) RejectResourceReconcilation(cluster cluster.Interface, gk schema.GroupKind) bool {
	return !this.responsible.Contains(resources.NewClusterGroupKind(cluster.GetId(), gk))
}

func (this *usageReconciler) Setup() error {
	if this.relations != nil {
		for from, rel := range this.relations.relations {
			c := this.Controller().GetClusterById(from.Cluster)
			for to, f := range rel.to {
				err := this.cache.SetupForRelation(this.Controller(), c, from.GroupKind, to, f)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (this *usageReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.cache.ExecuteActionForUsersOf(logger, "changed -> trigger", this.controller, obj.ClusterKey(), GlobalEnqueueAction)
	return reconcile.Succeeded(logger)
}

func (this *usageReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.cache.ExecuteActionForUsersOf(logger, "deleted -> trigger", this.controller, key, GlobalEnqueueAction)
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////

func UsageReconcilerForGKs(name string, cluster string, gks ...schema.GroupKind) controller.ConfigurationModifier {
	return func(c controller.Configuration) controller.Configuration {
		if c.Definition().Reconcilers()[name] == nil {
			c = c.Reconciler(CreateSimpleUsageReconcilerTypeFor(cluster, gks...), name)
		}
		return c.Cluster(cluster).ReconcilerWatchesByGK(name, gks...)
	}
}

// CreateSimpleUsageReconcilerTypeFor is deprecated, please use the usage relation variant
func CreateSimpleUsageReconcilerTypeFor(clusterName string, gks ...schema.GroupKind) controller.ReconcilerType {
	return func(controller controller.Interface) (reconcile.Interface, error) {
		cache := GetSharedSimpleUsageCache(controller)
		cluster := controller.GetCluster(clusterName)
		if cluster == nil {
			return nil, fmt.Errorf("cluster %s not found", clusterName)
		}
		cgks := resources.ClusterGroupKindSet{}
		for _, gk := range gks {
			cgks.Add(resources.NewClusterGroupKind(cluster.GetId(), gk))
		}
		this := &usageReconciler{
			ReconcilerSupport: NewReconcilerSupport(controller),
			cache:             cache,
			responsible:       cache.reconcilerFor(cgks),
		}
		return this, nil
	}
}

////////////////////////////////////////////////////////////////////////////////
// superior relation support

type controllerExtensionDefinition struct {
	relations []*UsageRelation
}

type ControllerUsageRelations struct {
	*SimpleUsageCache
	relations map[resources.ClusterGroupKind]*controllerUsageRelation
}

type controllerUsageRelation struct {
	from   resources.ClusterGroupKind                             // using resource kind
	to     map[resources.ClusterGroupKind]resources.UsedExtractor // used resources
	filter filter.KeyFilter                                       // resource filter for all used resources
}

func UsageRelationsForController(cntr controller.Interface) (*ControllerUsageRelations, error) {
	extdef := cntr.GetDefinition().GetDefinitionExtension(usersKey).(*controllerExtensionDefinition)

	cur := &ControllerUsageRelations{
		SimpleUsageCache: GetSharedSimpleUsageCache(cntr),
		relations:        map[resources.ClusterGroupKind]*controllerUsageRelation{},
	}
	// first gather all relations
	for _, d := range extdef.relations {
		cidFrom := cntr.GetCluster(d.FromCluster)
		if cidFrom == nil {
			return nil, fmt.Errorf("cluster id for cluster %q not found for controller %s", d.FromCluster, cntr.GetName())
		}
		cgk := resources.NewClusterGroupKind(cidFrom.GetId(), d.From)
		rel := cur.relations[cgk]
		if rel == nil {
			rel = &controllerUsageRelation{
				from:   cgk,
				to:     map[resources.ClusterGroupKind]resources.UsedExtractor{},
				filter: nil,
			}
			cur.relations[cgk] = rel
		}
		cidTo := cntr.GetCluster(d.ToCluster)
		if cidTo == nil {
			return nil, fmt.Errorf("cluster id for cluster %q not found for controller %s", d.ToCluster, cntr.GetName())
		}

		for _, r := range d.To {
			ukey := resources.NewClusterGroupKind(cidTo.GetId(), r.Resource)
			if rel.to[ukey] != nil {
				return nil, fmt.Errorf("multiple declarations for relation %s -> %s", cgk, ukey)
			}
			rel.to[ukey] = r.Extractor
		}
	}

	// second create filters
	cntr.Infof("found %d usage relations", len(cur.relations))
	for _, rel := range cur.relations {
		cgks := resources.ClusterGroupKindSet{}
		for u := range rel.to {
			cgks.Add(u)
		}
		rel.filter = filter.ClusterGroupKindFilterBySet(cgks)
		cntr.Infof("  %s -> %s", rel.from, cgks)
	}
	return cur, nil
}

func (this *ControllerUsageRelations) UsedResources() resources.ClusterGroupKindSet {
	used := resources.ClusterGroupKindSet{}
	for _, rel := range this.relations {
		for u := range rel.to {
			used.Add(u)
		}
	}
	return used
}

func (this *ControllerUsageRelations) UsersFor(user resources.Object) resources.ClusterObjectKeySet {
	used := resources.ClusterObjectKeySet{}
	rel := this.relations[user.ClusterKey().ClusterGroupKind()]
	if rel != nil {
		for _, f := range rel.to {
			used.AddSet(f(user))
		}
	}
	return used
}

// LockAndUpdateUsagesFor updates the usage of an object of a dedicated kind
// the used object is locked and an unlock function returned
func (this *ControllerUsageRelations) LockAndUpdateUsagesFor(user resources.Object) func() {
	cgk := user.ClusterKey().ClusterGroupKind()
	rel := this.relations[cgk]
	if rel == nil {
		panic(fmt.Sprintf("no usage relation for %s", cgk))
	}
	used := this.UsersFor(user)

	keys := used.AsArray()
	sort.Sort(keys)
	for _, key := range keys {
		this.Lock(nil, key)
	}
	this.UpdateFilteredUsesFor(user.ClusterKey(), rel.filter, used)
	return func() {
		for _, key := range keys {
			this.Unlock(key)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

type UsedResourceSpec struct {
	Resource  schema.GroupKind        // used resource
	Extractor resources.UsedExtractor // extractor for using resource
}

func UsedResource(gk schema.GroupKind, extractor resources.UsedExtractor) UsedResourceSpec {
	return UsedResourceSpec{gk, extractor}
}

type UsageRelation struct {
	FromCluster string             // cluster for From Resource
	From        schema.GroupKind   // using Resource
	ToCluster   string             // cluster for To Resource
	To          []UsedResourceSpec // used resources
}

func UsageRelationFor(fromCluster string, gk schema.GroupKind, toCluster string, specs ...UsedResourceSpec) *UsageRelation {
	return &UsageRelation{
		FromCluster: fromCluster,
		From:        gk,
		ToCluster:   toCluster,
		To:          specs,
	}
}

func UsageRelationForGK(fromCluster string, gk schema.GroupKind, toCluster string, used schema.GroupKind, extractor resources.UsedExtractor) *UsageRelation {
	return UsageRelationFor(fromCluster, gk, toCluster, UsedResource(used, extractor))
}

func MainClusterUsageRelationFor(gk schema.GroupKind, specs ...UsedResourceSpec) *UsageRelation {
	return UsageRelationFor(controller.CLUSTER_MAIN, gk, controller.CLUSTER_MAIN, specs...)
}

func MainClusterUsageRelationForGK(gk schema.GroupKind, used schema.GroupKind, extractor resources.UsedExtractor) *UsageRelation {
	return UsageRelationFor(controller.CLUSTER_MAIN, gk, controller.CLUSTER_MAIN, UsedResource(used, extractor))
}

func (this *UsageRelation) GKs() []schema.GroupKind {
	gks := []schema.GroupKind{}
	for _, r := range this.To {
		gks = append(gks, r.Resource)
	}
	return gks
}

func UsageReconcilerForRelation(name string, relation *UsageRelation) controller.ConfigurationModifier {
	return func(c controller.Configuration) controller.Configuration {
		c, ext := c.AssureDefinitionExtension(usersKey, func() interface{} { return &controllerExtensionDefinition{} })
		extdef := ext.(*controllerExtensionDefinition)
		extdef.relations = append(extdef.relations, relation)
		if c.Definition().Reconcilers()[name] == nil {
			c = c.Reconciler(createSimpleUsageReconciler, name)
		}
		return c.Cluster(relation.FromCluster).ReconcilerWatchesByGK(name, relation.GKs()...)
	}
}

func createSimpleUsageReconciler(controller controller.Interface) (reconcile.Interface, error) {
	rel, err := UsageRelationsForController(controller)
	if err != nil {
		return nil, err
	}
	this := &usageReconciler{
		ReconcilerSupport: NewReconcilerSupport(controller),
		cache:             rel.SimpleUsageCache,
		responsible:       rel.reconcilerFor(rel.UsedResources()),
		relations:         rel,
	}
	return this, nil
}

////////////////////////////////////////////////////////////////////////////////

type ProcessingFunction func(logger logger.LogContext, obj resources.Object) (bool, error)

func ProcessResource(logger logger.LogContext, action string, resc resources.Interface, process ProcessingFunction) error {
	if logger != nil {
		logger.Infof("%s %s", action, resc.Name())
	}
	list, err := resc.ListCached(labels.Everything())
	if err == nil {
		for _, l := range list {
			handled, err := process(logger, l)
			if err != nil {
				logger.Infof("  errorneous %s %s: %s", resc.Name(), l.ObjectName(), err)
				if !handled {
					return err
				}
			} else {
				if handled {
					logger.Infof("  found %s %s", resc.Name(), l.ObjectName())
				}
			}
		}
	}
	return err
}
