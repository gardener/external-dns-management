// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "service-source"

// Actuator is the actuator for Service source objects.
type Actuator struct {
}

var _ common.SourceActuator[*corev1.Service] = &Actuator{}

// ReconcileSourceObject reconciles the given Service object.
func (a *Actuator) ReconcileSourceObject(
	ctx context.Context,
	r *common.SourceReconciler[*corev1.Service],
	service *corev1.Service,
) (
	reconcile.Result,
	error,
) {
	r.Log.Info("reconcile")

	var input *common.DNSSpecInput
	if a.IsRelevantSourceObject(r, service) {
		var err error
		input, err = common.GetDNSSpecInputForService(r.Log, r.State, r.GVK, service)
		if err != nil {
			r.Recorder.DedupEventf(service, corev1.EventTypeWarning, "Invalid", "%s", err)
			return reconcile.Result{}, err
		}
	}

	res, err := r.DoReconcile(ctx, service, input)
	if err != nil {
		r.Recorder.DedupEventf(service, corev1.EventTypeWarning, "ReconcileError", "%s", err)
	}
	return res, err
}

// ControllerName returns the name of this controller.
func (a *Actuator) ControllerName() string {
	return ControllerName
}

// FinalizerLocalName returns the local name of the finalizer.
func (a *Actuator) FinalizerLocalName() string {
	return "service-dns"
}

// GetGVK returns the GVK of Service.
func (a *Actuator) GetGVK() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Service")
}

// IsRelevantSourceObject checks whether the given Service is relevant for DNS management.
func (a *Actuator) IsRelevantSourceObject(r *common.SourceReconciler[*corev1.Service], svc *corev1.Service) bool {
	if svc == nil {
		return false
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return false
	}

	annotations := common.GetMergedAnnotation(r.GVK, r.State, svc)
	if !dns.EquivalentClass(annotations[dns.AnnotationClass], r.SourceClass) {
		return false
	}
	_, ok := annotations[dns.AnnotationDNSNames]
	return ok
}

// NewSourceObject creates a new Service object.
func (a *Actuator) NewSourceObject() *corev1.Service {
	return &corev1.Service{}
}

// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
func (a *Actuator) ShouldSetTargetEntryAnnotation() bool {
	return false
}

// OnDelete is called when a Service is deleted. No action is needed in this case.
func (a *Actuator) OnDelete(_ *corev1.Service) {}
