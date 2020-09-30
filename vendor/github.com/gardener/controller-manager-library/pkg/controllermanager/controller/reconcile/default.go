/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcile

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type DefaultReconciler struct {
}

func (r *DefaultReconciler) Reconcile(logger logger.LogContext, obj resources.Object) Status {
	return Succeeded(logger)
}
func (r *DefaultReconciler) Delete(logger logger.LogContext, obj resources.Object) Status {
	return Succeeded(logger)
}
func (r *DefaultReconciler) Deleted(logger logger.LogContext, obj resources.ClusterObjectKey) Status {
	return Succeeded(logger)
}
func (r *DefaultReconciler) Command(logger logger.LogContext, cmd string) Status {
	return Failed(logger, fmt.Errorf("unknown command %q", cmd))
}
