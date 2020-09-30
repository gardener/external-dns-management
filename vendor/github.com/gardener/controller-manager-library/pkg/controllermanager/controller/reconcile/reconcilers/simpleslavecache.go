/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package reconcilers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

var slavesKey = ctxutil.SimpleKey("slaves")

// GetSharedSimpleSlaveCache returns an instance of a usage cache unique for
// the complete controller manager
func GetSharedSimpleSlaveCache(controller controller.Interface) *SimpleSlaveCache {
	mig := controller.GetEnvironment().ControllerManager().GetClusterIdMigration()
	return controller.GetEnvironment().GetOrCreateSharedValue(slavesKey, func() interface{} {
		return NewSimpleSlaveCache(mig)
	}).(*SimpleSlaveCache)
}

type SimpleSlaveCache struct {
	migration resources.ClusterIdMigration
	usages    *SimpleUsageCache
}

func NewSimpleSlaveCache(migration ...resources.ClusterIdMigration) *SimpleSlaveCache {
	var mig resources.ClusterIdMigration
	if len(migration) > 0 {
		mig = migration[0]
	}
	return &SimpleSlaveCache{
		migration: mig,
		usages:    NewSimpleUsageCache(),
	}
}

func (this *SimpleSlaveCache) GetOwnersFor(name resources.ClusterObjectKey, filter resources.KeyFilter) resources.ClusterObjectKeySet {
	return this.usages.GetFilteredUsesFor(name, filter)
}

func (this *SimpleSlaveCache) GetSlavesFor(name resources.ClusterObjectKey, filter resources.KeyFilter) resources.ClusterObjectKeySet {
	return this.usages.GetFilteredUsersFor(name, filter)
}

func (this *SimpleSlaveCache) CreateSlaveFor(obj resources.Object, slave resources.Object) error {
	slave.AddOwner(obj)
	err := slave.Create()
	if err == nil {
		this.usages.UpdateUsesFor(slave.ClusterKey(), slave.GetOwners())
	}
	return err
}

func (this *SimpleSlaveCache) NotifySlaveModification(log logger.LogContext, controller controller.Interface, slave resources.ClusterObjectKey, filter resources.KeyFilter) error {
	return this.ExecuteActionForOwnersOf(log, "%s changed -> trigger owners", controller, slave, filter, GlobalEnqueueAction)
}

func (this *SimpleSlaveCache) UpdateSlave(slave resources.Object) {
	this.usages.UpdateUsesFor(slave.ClusterKey(), slave.GetOwners())
}

func (this *SimpleSlaveCache) SetupFor(log logger.LogContext, resc resources.Interface) error {
	return ProcessResource(log, "setup owners for", resc, func(log logger.LogContext, obj resources.Object) (bool, error) {
		if this.migration != nil {
			err := resources.MigrateOwnerClusterIds(obj, this.migration)
			if err != nil {
				return false, err
			}
		}
		this.UpdateSlave(obj)
		return true, nil
	})
}

func (this *SimpleSlaveCache) DeleteSlave(log logger.LogContext, msg string, controller controller.Interface, slave resources.ClusterObjectKey, actions ...KeyAction) error {
	return this.usages.CleanupUser(log, msg, controller, slave, actions...)
}

func (this *SimpleSlaveCache) ExecuteActionForOwnersOf(log logger.LogContext, msg string, controller controller.Interface, slave resources.ClusterObjectKey, filter resources.KeyFilter, actions ...KeyAction) error {
	if len(actions) > 0 {
		used := this.GetOwnersFor(slave, filter)
		if len(used) > 0 && log != nil && msg != "" {
			log.Infof("%s owners of %s", msg, slave.ObjectKey())
		}
		for key := range used {
			for _, a := range actions {
				err := a(log, controller, key)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (this *SimpleSlaveCache) ExecuteActionForSlavesOf(log logger.LogContext, msg string, controller controller.Interface, owner resources.ClusterObjectKey, filter resources.KeyFilter, actions ...KeyAction) error {
	if len(actions) > 0 {
		used := this.GetSlavesFor(owner, filter)
		if len(used) > 0 && log != nil && msg != "" {
			log.Infof("%s slaves of %s", msg, owner.ObjectKey())
		}
		for key := range used {
			for _, a := range actions {
				err := a(log, controller, key)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type slaveReconciler struct {
	ReconcilerSupport
	cache       *SimpleSlaveCache
	clusterId   string
	responsible resources.GroupKindSet
}

var _ reconcile.Interface = &slaveReconciler{}
var _ reconcile.ReconcilationRejection = &slaveReconciler{}

func (this *slaveReconciler) RejectResourceReconcilation(cluster cluster.Interface, gk schema.GroupKind) bool {
	if cluster == nil || this.clusterId != cluster.GetId() {
		return true
	}
	return !this.responsible.Contains(gk)
}

func (this *slaveReconciler) Setup() error {
	for r := range this.responsible {
		res, err := this.controller.GetClusterById(this.clusterId).Resources().Get(r)
		if err != nil {
			return fmt.Errorf("cannot find resource %s on cluster %s: %s", r, this.clusterId, err)
		}
		this.cache.SetupFor(this.controller, res)
	}
	return nil
}

func (this *slaveReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.cache.ExecuteActionForOwnersOf(logger, "changed -> trigger", this.controller, obj.ClusterKey(), nil, GlobalEnqueueAction)
	return reconcile.Succeeded(logger)
}

func (this *slaveReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.cache.DeleteSlave(logger, "deleted -> trigger", this.controller, key, GlobalEnqueueAction)
	return reconcile.Succeeded(logger)
}

////////////////////////////////////////////////////////////////////////////////

func SlaveReconcilerForGKs(name string, cluster string, gks ...schema.GroupKind) controller.ConfigurationModifier {
	return func(c controller.Configuration) controller.Configuration {
		if c.Definition().Reconcilers()[name] == nil {
			c = c.Reconciler(CreateSimpleSlaveReconcilerTypeFor(cluster, gks...), name)
		}
		return c.Cluster(cluster).ReconcilerWatchesByGK(name, gks...)
	}
}

func CreateSimpleSlaveReconcilerTypeFor(clusterName string, gks ...schema.GroupKind) controller.ReconcilerType {
	return func(controller controller.Interface) (reconcile.Interface, error) {
		cache := GetSharedSimpleSlaveCache(controller)
		cluster := controller.GetCluster(clusterName)
		if cluster == nil {
			return nil, fmt.Errorf("cluster %s not found", clusterName)
		}
		this := &slaveReconciler{
			ReconcilerSupport: NewReconcilerSupport(controller),
			cache:             cache,
			clusterId:         cluster.GetId(),
			responsible:       cache.usages.reconcilerFor(cluster, gks...),
		}
		return this, nil
	}
}
