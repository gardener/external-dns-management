/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"k8s.io/client-go/tools/cache"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

func convert(resource Interface, funcs *ResourceEventHandlerFuncs) *cache.ResourceEventHandlerFuncs {
	return &cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			o, err := resource.Wrap(obj.(ObjectData))
			if err == nil {
				funcs.AddFunc(o)
			}
		},
		DeleteFunc: func(obj interface{}) {
			data, ok := obj.(ObjectData)
			if !ok {
				stale, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					logger.Errorf("informer %q reported unknown object to be deleted (%T)", resource.Name(), obj)
					return
				}
				if stale.Obj == nil {
					logger.Errorf("informer %q reported no stale object to be deleted", resource.Name())
					return
				}
				data, ok = stale.Obj.(ObjectData)
				if !ok {
					logger.Errorf("informer %q reported unknown stale object to be deleted (%T)", resource.Name(), stale.Obj)
					return
				}
			}
			o, err := resource.Wrap(data)
			if err == nil {
				funcs.DeleteFunc(o)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			o, err := resource.Wrap(old.(ObjectData))
			if err == nil {
				n, err := resource.Wrap(new.(ObjectData))
				if err == nil {
					funcs.UpdateFunc(o, n)
				}
			}
		},
	}
}
