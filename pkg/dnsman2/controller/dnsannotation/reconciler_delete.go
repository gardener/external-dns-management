// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsanntation

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) delete(_ context.Context, _ logr.Logger, annotation *v1alpha1.DNSAnnotation) (reconcile.Result, error) {
	r.state.DeleteResourceAnnotations(annotation.Spec.ResourceRef)
	return reconcile.Result{}, nil
}

func (r *Reconciler) deleteByKey(nameKey client.ObjectKey) {
	r.state.DeleteByAnnotationKey(nameKey)
}
