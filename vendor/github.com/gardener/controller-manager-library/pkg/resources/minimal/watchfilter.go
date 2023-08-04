/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package minimal

import (
	"github.com/gardener/controller-manager-library/pkg/kutil"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

func init() {
	kutil.AddGenericType(&metav1.PartialObjectMetadata{}, &metav1.PartialObjectMetadataList{})
}

// MinimalWatchFilter creates a Filter watch returning watchedObjects
func MinimalWatchFilter(w watch.Interface) watch.Interface {
	return watch.Filter(w, convertEventToPartialObjectMetadata)
}

var ConvertCounter = 0

func ConvertToPartialObjectMetadata(apiVersion, kind string, metaObj metav1.Object) *metav1.PartialObjectMetadata {
	obj := meta.AsPartialObjectMetadata(metaObj)
	obj.APIVersion = apiVersion
	obj.Kind = kind
	return obj
}

func convertEventToPartialObjectMetadata(evt watch.Event) (watch.Event, bool) {
	if meta, ok := evt.Object.(metav1.Object); ok {
		apiVersion, kind := evt.Object.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
		evt.Object = ConvertToPartialObjectMetadata(apiVersion, kind, meta)
		ConvertCounter++
	}
	return evt, true
}
