/*
 * Copyright 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

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
