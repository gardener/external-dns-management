// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetAnnotation sets the given annotation key to the specified value on the provided object.
func SetAnnotation(obj metav1.Object, key, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
}

// RemoveAnnotation removes the given annotation key from the provided object.
func RemoveAnnotation(obj metav1.Object, key string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return
	}
	delete(annotations, key)
	obj.SetAnnotations(annotations)
}

// SetLabel sets the given label key to the specified value on the provided object.
func SetLabel(obj metav1.Object, key, value string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[key] = value
	obj.SetLabels(labels)
}

// RemoveLabel removes the given label key from the provided object.
func RemoveLabel(obj metav1.Object, key string) {
	labels := obj.GetLabels()
	if labels == nil {
		return
	}
	delete(labels, key)
	obj.SetLabels(labels)
}
