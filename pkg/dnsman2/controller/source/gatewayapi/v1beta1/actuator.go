// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "gatewayapiv1beta1-source"

// Actuator is an actuator for provided Gateway resources.
type Actuator struct {
	Discovery discovery.DiscoveryInterface
}

var (
	// Activated indicates whether this source controller is activated.
	Activated bool
	_         common.SourceActuator[*gatewayapisv1beta1.Gateway] = &Actuator{}
)

// ReconcileSourceObject reconciles the given Gateway resource.
func (a *Actuator) ReconcileSourceObject(
	ctx context.Context,
	r *common.SourceReconciler[*gatewayapisv1beta1.Gateway],
	gateway *gatewayapisv1beta1.Gateway,
) (
	reconcile.Result,
	error,
) {
	r.Log.Info("reconcile")

	var input *common.DNSSpecInput
	if a.IsRelevantSourceObject(r, gateway) {
		var err error
		input, err = gatewayapi.GetDNSSpecInput(ctx, r, gateway)
		if err != nil {
			r.Recorder.DedupEventf(gateway, corev1.EventTypeWarning, "Invalid", "%s", err)
			return reconcile.Result{}, err
		}
	}

	res, err := r.DoReconcile(ctx, gateway, input)
	if err != nil {
		r.Recorder.DedupEventf(gateway, corev1.EventTypeWarning, "ReconcileError", "%s", err)
	}
	return res, err
}

// ControllerName returns the name of this controller.
func (a *Actuator) ControllerName() string {
	return ControllerName
}

// FinalizerLocalName returns the local name of the finalizer for Gateway resources.
func (a *Actuator) FinalizerLocalName() string {
	return "gatewayapi-dns"
}

// GetGVK returns the GVK of Gateway resources.
func (a *Actuator) GetGVK() schema.GroupVersionKind {
	return gatewayapi.GetGVKV1beta1()
}

// IsRelevantSourceObject checks whether the given Gateway resource is relevant for processing.
func (a *Actuator) IsRelevantSourceObject(r *common.SourceReconciler[*gatewayapisv1beta1.Gateway], gateway *gatewayapisv1beta1.Gateway) bool {
	if gateway == nil {
		return false
	}
	annotations := common.GetMergedAnnotation(r.GVK, r.State, gateway)
	if !dns.EquivalentClass(annotations[dns.AnnotationClass], r.SourceClass) {
		return false
	}
	_, ok := annotations[dns.AnnotationDNSNames]
	return ok
}

// NewSourceObject creates a new Gateway resource.
func (a *Actuator) NewSourceObject() *gatewayapisv1beta1.Gateway {
	return &gatewayapisv1beta1.Gateway{}
}

// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
func (a *Actuator) ShouldSetTargetEntryAnnotation() bool {
	return false
}

// ShouldActivate checks whether the required Gateway API v1beta1 CRDs are present in the cluster.
// If the v1 CRDs are present, the v1beta1 controller should be deactivated in favor of the v1 controller and it returns false.
func (a *Actuator) ShouldActivate() (bool, error) {
	apiVersion, err := gatewayapi.DetermineAPIVersion(a.Discovery)
	if err != nil {
		return false, err
	}
	return apiVersion != nil && *apiVersion == gatewayapi.V1Beta1, nil
}

// WatchHTTPRoutes adds a watch for HTTPRoute resources to the given builder that maps them to Gateway reconciliation requests.
func (a *Actuator) WatchHTTPRoutes(b *builder.Builder) *builder.Builder {
	return b.Watches(
		&gatewayapisv1beta1.HTTPRoute{},
		handler.EnqueueRequestsFromMapFunc(a.mapHTTPRouteToGatewayRequest),
	)
}

func (a *Actuator) mapHTTPRouteToGatewayRequest(_ context.Context, obj client.Object) []reconcile.Request {
	route, ok := obj.(*gatewayapisv1beta1.HTTPRoute)
	if !ok {
		return nil
	}
	gatewayKeys := gatewayapi.ExtractGatewayKeys(a.GetGVK(), (*gatewayapisv1.HTTPRoute)(route))
	requests := make([]reconcile.Request, len(gatewayKeys))
	for i, gatewayKey := range gatewayKeys {
		requests[i] = reconcile.Request{NamespacedName: gatewayKey}
	}
	return requests
}
