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

package reconcilers

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type NestedReconciler struct {
	nested reconcile.Interface
}

var _ reconcile.Interface = &NestedReconciler{}

func NewNestedReconciler(reconciler controller.ReconcilerType, c controller.Interface) (*NestedReconciler, error) {
	if reconciler != nil {
		r, err := reconciler(c)
		if err != nil {
			return nil, fmt.Errorf("cannot created nested reconciler: %s", err)
		}
		return &NestedReconciler{r}, nil
	}
	return &NestedReconciler{}, nil
}

func (this *NestedReconciler) Setup() {
	if this.nested != nil {
		this.nested.Setup()
	}
}

func (this *NestedReconciler) Start() {
	if this.nested != nil {
		this.nested.Start()
	}
}

func (this *NestedReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if this.nested != nil {
		return this.nested.Reconcile(logger, obj)
	}
	return reconcile.Succeeded(logger)
}

func (this *NestedReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if this.nested != nil {
		return this.nested.Delete(logger, obj)
	}
	return reconcile.Succeeded(logger)
}

func (this *NestedReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	if this.nested != nil {
		return this.nested.Deleted(logger, key)
	}
	return reconcile.Succeeded(logger)
}

func (this *NestedReconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {
	if this.nested != nil {
		return this.nested.Command(logger, cmd)
	}
	return reconcile.Failed(logger, fmt.Errorf("unknown command %q", cmd))
}
