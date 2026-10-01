// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// NiceRemoveFinalizers removes the given finalizers from the provided object.
// It is a no-op if the object contains none of the given finalizers, avoiding
// an unnecessary patch request.
func NiceRemoveFinalizers(ctx context.Context, writer client.Writer, obj client.Object, finalizers ...string) error {
	needRemoveFinalizers := false
	for _, finalizer := range finalizers {
		if controllerutil.ContainsFinalizer(obj, finalizer) {
			needRemoveFinalizers = true
			break
		}
	}
	if !needRemoveFinalizers {
		return nil
	}
	return controllerutils.RemoveFinalizers(ctx, writer, obj, finalizers...)
}
