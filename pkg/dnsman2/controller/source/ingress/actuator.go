// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ingress

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "ingress-source"

// Actuator is an actuator for provided Ingress resources.
type Actuator struct {
}

var _ common.SourceActuator[*networkingv1.Ingress] = &Actuator{}

// ReconcileSourceObject reconciles the given Ingress resource.
func (a *Actuator) ReconcileSourceObject(
	ctx context.Context,
	r *common.SourceReconciler[*networkingv1.Ingress],
	ingress *networkingv1.Ingress,
) (
	reconcile.Result,
	error,
) {
	r.Log.Info("reconcile")

	var input *common.DNSSpecInput
	if a.IsRelevantSourceObject(r, ingress) {
		var err error
		input, err = common.GetDNSSpecInputForIngress(r.Log, r.State, r.GVK, ingress)
		if err != nil {
			r.Recorder.DedupEventf(ingress, corev1.EventTypeWarning, "Invalid", "%s", err)
			return reconcile.Result{}, err
		}
	}

	res, err := r.DoReconcile(ctx, ingress, input)
	if err != nil {
		r.Recorder.DedupEventf(ingress, corev1.EventTypeWarning, "ReconcileError", "%s", err)
	}
	return res, err
}

// ControllerName returns the name of this controller.
func (a *Actuator) ControllerName() string {
	return ControllerName
}

// FinalizerLocalName returns the local name of the finalizer for Ingress resources.
func (a *Actuator) FinalizerLocalName() string {
	return "ingress-dns"
}

// GetGVK returns the GVK of Ingress resources.
func (a *Actuator) GetGVK() schema.GroupVersionKind {
	return networkingv1.SchemeGroupVersion.WithKind("Ingress")
}

// IsRelevantSourceObject checks whether the given Ingress resource is relevant for processing.
func (a *Actuator) IsRelevantSourceObject(r *common.SourceReconciler[*networkingv1.Ingress], ing *networkingv1.Ingress) bool {
	if ing == nil {
		return false
	}
	annotations := common.GetMergedAnnotation(r.GVK, r.State, ing)
	if !dns.EquivalentClass(annotations[dns.AnnotationClass], r.SourceClass) {
		return false
	}
	_, ok := annotations[dns.AnnotationDNSNames]
	return ok
}

// NewSourceObject creates a new Ingress resource.
func (a *Actuator) NewSourceObject() *networkingv1.Ingress {
	return &networkingv1.Ingress{}
}

// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
func (a *Actuator) ShouldSetTargetEntryAnnotation() bool {
	return false
}
