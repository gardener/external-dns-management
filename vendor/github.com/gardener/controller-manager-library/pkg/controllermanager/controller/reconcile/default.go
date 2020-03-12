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

package reconcile

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type DefaultReconciler struct {
}

func (r *DefaultReconciler) Setup() {
}

func (r *DefaultReconciler) Start() {
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
