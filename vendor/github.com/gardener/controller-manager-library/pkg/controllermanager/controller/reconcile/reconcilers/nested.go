/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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

func (this *NestedReconciler) Setup() error {
	if this.nested != nil {
		return reconcile.SetupReconciler(this.nested)
	}
	return nil
}

func (this *NestedReconciler) Start() error {
	if this.nested != nil {
		return reconcile.StartReconciler(this.nested)
	}
	return nil
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
