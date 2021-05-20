/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package minimal

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// MinimalWatchFilter creates a Filter watch returning watchedObjects
func MinimalWatchFilter(w watch.Interface) watch.Interface {
	return watch.Filter(w, convertEventToMinimalObject)
}

var ConvertCounter = 0

func ConvertToMinimalObject(apiVersion, kind string, meta metav1.Object) *MinimalObject {
	return &MinimalObject{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: apiVersion,
		},
		MinimalMeta: MinimalMeta{
			Namespace:         meta.GetNamespace(),
			Name:              meta.GetName(),
			UID:               meta.GetUID(),
			ResourceVersion:   meta.GetResourceVersion(),
			Generation:        meta.GetGeneration(),
			CreationTimestamp: meta.GetCreationTimestamp(),
			DeletionTimestamp: meta.GetDeletionTimestamp(),
			Labels:            meta.GetLabels(),
		},
	}
}

func convertEventToMinimalObject(evt watch.Event) (watch.Event, bool) {
	if meta, ok := evt.Object.(metav1.Object); ok {
		apiVersion, kind := evt.Object.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
		evt.Object = ConvertToMinimalObject(apiVersion, kind, meta)
		ConvertCounter++
	}
	return evt, true
}
