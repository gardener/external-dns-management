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

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type ReconcilerSupport struct {
	reconcile.DefaultReconciler
	controller controller.Interface
}

func NewReconcilerSupport(c controller.Interface) ReconcilerSupport {
	return ReconcilerSupport{controller: c}
}

func (this *ReconcilerSupport) Controller() controller.Interface {
	return this.controller
}

func (this *ReconcilerSupport) EnqueueKeys(keys resources.ClusterObjectKeySet) {
	for key := range keys {
		this.Controller().EnqueueKey(key)
	}
}

func (this *ReconcilerSupport) EnqueueObject(gk schema.GroupKind, name resources.ObjectName, cluster ...string) error {
	key := this.NewClusterObjectKey(gk, name, cluster...)
	if key.Cluster() == "" {
		return fmt.Errorf("unknown cluster")
	}
	return this.controller.EnqueueKey(key)
}

func (this *ReconcilerSupport) EnqueueObjectReferencedBy(obj resources.Object, gk schema.GroupKind, name resources.ObjectName) error {
	key := resources.NewClusterKey(obj.GetCluster().GetId(), gk, name.Namespace(), name.Name())
	return this.controller.EnqueueKey(key)
}

func (this *ReconcilerSupport) NewClusterObjectKey(gk schema.GroupKind, name resources.ObjectName, cluster ...string) resources.ClusterObjectKey {
	if len(cluster) == 0 {
		return resources.NewClusterKey(this.Controller().GetMainCluster().GetId(), gk, name.Namespace(), name.Name())
	}
	c := this.controller.GetCluster(cluster[0])
	if c == nil {
		return resources.NewClusterKey("", gk, name.Namespace(), name.Name())
	}
	return resources.NewClusterKey(c.GetId(), gk, name.Namespace(), name.Name())
}
