/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package reconcilers

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

// GetSharedReconciler returns an instance of a reconciler unique for
// the complete controller manager
func GetSharedReconciler(t SharedReconcilerType, controller controller.Interface) *SharedReconciler {
	return controller.GetEnvironment().GetOrCreateSharedValue(t, func() interface{} {
		return NewSharedReconciler(t)
	}).(*SharedReconciler)
}

type SharedReconciler struct {
	lock        sync.Mutex
	state       interface{}
	reconcilers controller.WatchedResources
}

func NewSharedReconciler(t SharedReconcilerType) *SharedReconciler {
	return &SharedReconciler{
		reconcilers: controller.WatchedResources{},
		state:       t.CreateState(),
	}
}

// reconcilerFor is used to assure that only one reconciler in one controller
// handles the reconcilations. The shared info is hold at controller
// extension level and is shared among all controllers of a controller manager.
func (this *SharedReconciler) reconcilerFor(cluster cluster.Interface, gks ...schema.GroupKind) resources.GroupKindSet {
	responsible := resources.GroupKindSet{}

	this.lock.Lock()
	defer this.lock.Unlock()
	this.reconcilers.GatheredAdd(cluster.GetId(), responsible, gks...)
	return responsible
}

func (this *SharedReconciler) State() interface{} {
	return this.state
}

////////////////////////////////////////////////////////////////////////////////

type sharedReconciler struct {
	shared      *SharedReconciler
	reconciler  reconcile.Interface
	clusterId   string
	responsible resources.GroupKindSet
}

var _ reconcile.Interface = &sharedReconciler{}
var _ reconcile.ReconcilationRejection = &sharedReconciler{}

func (this *sharedReconciler) RejectResourceReconcilation(cluster cluster.Interface, gk schema.GroupKind) bool {
	if cluster == nil || this.clusterId != cluster.GetId() {
		return true
	}
	return !this.responsible.Contains(gk)
}

func (this *sharedReconciler) Setup() error {
	return reconcile.SetupReconciler(this.reconciler)
}
func (this *sharedReconciler) Start() error {
	return reconcile.StartReconciler(this.reconciler)
}

func (this *sharedReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	return this.reconciler.Reconcile(logger, obj)
}
func (this *sharedReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	return this.reconciler.Delete(logger, obj)
}
func (this *sharedReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	return this.reconciler.Deleted(logger, key)
}
func (this *sharedReconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {
	return this.reconciler.Command(logger, cmd)
}

////////////////////////////////////////////////////////////////////////////////

type SharedReconcilerType interface {
	CreateReconciler(controller controller.Interface, state interface{}) (reconcile.Interface, error)
	CreateState() interface{}
}

func NewSharedReconcilerType(r func(controller controller.Interface, state interface{}) (reconcile.Interface, error),
	s func() interface{}) SharedReconcilerType {
	return &sharedReconcilerType{
		reconcilerType: r,
		state:          s,
	}
}

type sharedReconcilerType struct {
	reconcilerType func(controller controller.Interface, state interface{}) (reconcile.Interface, error)
	state          func() interface{}
}

func (this *sharedReconcilerType) CreateReconciler(controller controller.Interface, state interface{}) (reconcile.Interface, error) {
	return this.reconcilerType(controller, state)
}

func (this *sharedReconcilerType) CreateState() interface{} {
	return this.state()
}

////////////////////////////////////////////////////////////////////////////////

func SharedReconcilerForGKs(name string, cluster string, reconcilerType SharedReconcilerType,
	gks ...schema.GroupKind) controller.ConfigurationModifier {
	return func(c controller.Configuration) controller.Configuration {
		if c.Definition().Reconcilers()[name] == nil {
			c = c.Reconciler(CreateSharedReconcilerTypeFor(cluster, reconcilerType, gks...), name)
		}
		return c.Cluster(cluster).ReconcilerWatchesByGK(name, gks...)
	}
}

func CreateSharedReconcilerTypeFor(clusterName string, reconcilerType SharedReconcilerType, gks ...schema.GroupKind) controller.ReconcilerType {
	return func(controller controller.Interface) (reconcile.Interface, error) {
		shared := GetSharedReconciler(reconcilerType, controller)
		cluster := controller.GetCluster(clusterName)
		if cluster == nil {
			return nil, fmt.Errorf("cluster %s not found", clusterName)
		}
		reconciler, err := reconcilerType.CreateReconciler(controller, shared.State())
		if err != nil {
			return nil, err
		}
		this := &sharedReconciler{
			shared:      shared,
			reconciler:  reconciler,
			clusterId:   cluster.GetId(),
			responsible: shared.reconcilerFor(cluster, gks...),
		}
		return this, nil
	}
}
