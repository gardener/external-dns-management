// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

func newTargetSourcesReconciler(c controller.Interface) (reconcile.Interface, error) {
	state, err := getOrCreateSharedState(c)
	if err != nil {
		return nil, err
	}
	return &targetSourcesReconciler{controller: c, state: state}, nil
}

var _ reconcile.Interface = &targetSourcesReconciler{}

type targetSourcesReconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface
	state      *resourcesState
}

func (r *targetSourcesReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	r.triggerGateways(obj.ClusterKey())
	return reconcile.Succeeded(logger)
}

func (r *targetSourcesReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	r.triggerGateways(obj.ClusterKey())
	return reconcile.Succeeded(logger)
}

func (r *targetSourcesReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	r.triggerGateways(key)
	return reconcile.Succeeded(logger)
}

func (r *targetSourcesReconciler) triggerGateways(source resources.ClusterObjectKey) {
	gateways := r.state.MatchingGatewaysByTargetSource(source.ObjectKey())
	for _, g := range gateways {
		_ = r.controller.EnqueueKey(resources.NewClusterKeyForObject(source.Cluster(), g.ForGroupKind(GroupKindGateway)))
	}
}
