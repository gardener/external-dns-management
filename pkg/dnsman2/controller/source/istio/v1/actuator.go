// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"

	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ControllerName is the name of this controller.
const ControllerName = "istiov1-source"

// Actuator is an actuator for provided Istio v1 Gateway resources.
type Actuator struct {
	Discovery discovery.DiscoveryInterface

	state *istio.ObjectToGatewaysState
}

var (
	// Activated indicates whether this source controller is activated.
	Activated bool
	_         common.SourceActuator[*istionetworkingv1.Gateway] = &Actuator{}
)

// NewActuator creates a new Actuator for Istio v1 Gateway resources.
func NewActuator(dc discovery.DiscoveryInterface) *Actuator {
	return &Actuator{
		Discovery: dc,
		state:     istio.NewObjectToGatewaysState(),
	}
}

// ReconcileSourceObject reconciles the given Istio v1 Gateway resource.
func (a *Actuator) ReconcileSourceObject(
	ctx context.Context,
	r *common.SourceReconciler[*istionetworkingv1.Gateway],
	gateway *istionetworkingv1.Gateway,
) (
	reconcile.Result,
	error,
) {
	r.Log.Info("reconcile")

	var input *common.DNSSpecInput
	if a.IsRelevantSourceObject(r, gateway) {
		var err error
		input, err = istio.GetDNSSpecInput(ctx, r, gateway, a.state)
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

// FinalizerLocalName returns the local name of the finalizer for Istio v1 Gateway resources.
func (a *Actuator) FinalizerLocalName() string {
	return "istio-dns"
}

// GetGVK returns the GVK of Gateway resources.
func (a *Actuator) GetGVK() schema.GroupVersionKind {
	return istionetworkingv1.SchemeGroupVersion.WithKind("Gateway")
}

// IsRelevantSourceObject checks whether the given Istio v1 Gateway resource is relevant for processing.
func (a *Actuator) IsRelevantSourceObject(r *common.SourceReconciler[*istionetworkingv1.Gateway], gateway *istionetworkingv1.Gateway) bool {
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

// NewSourceObject creates a new Istio v1 Gateway resource.
func (a *Actuator) NewSourceObject() *istionetworkingv1.Gateway {
	return &istionetworkingv1.Gateway{}
}

// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
func (a *Actuator) ShouldSetTargetEntryAnnotation() bool {
	return false
}

// OnDelete is called when an Istio v1 Gateway resource is deleted.
func (a *Actuator) OnDelete(gateway *istionetworkingv1.Gateway) {
	a.state.RemoveGateway(gateway)
}

// ShouldActivate checks whether the required Istio v1 CRDs are present in the cluster.
func (a *Actuator) ShouldActivate() (bool, error) {
	apiVersion, err := istio.DetermineAPIVersion(a.Discovery)
	if err != nil {
		return false, err
	}
	return apiVersion != nil && *apiVersion == istio.V1, nil
}

// WatchRelatedResources adds watches for Istio v1 VirtualService, Ingress, and Service to the given builder that maps them to Istio v1 Gateway reconciliation requests.
func (a *Actuator) WatchRelatedResources(b *builder.Builder) *builder.Builder {
	return b.Watches(
		&istionetworkingv1.VirtualService{},
		handler.EnqueueRequestsFromMapFunc(a.mapVirtualServiceToGatewayRequest),
	).Watches(
		&networkingv1.Ingress{},
		handler.EnqueueRequestsFromMapFunc(a.mapIngressToGatewayRequest),
	).Watches(
		&corev1.Service{},
		handler.EnqueueRequestsFromMapFunc(a.mapServiceToGatewayRequest),
	)
}

func (a *Actuator) mapVirtualServiceToGatewayRequest(_ context.Context, obj client.Object) []reconcile.Request {
	virtualService, ok := obj.(*istionetworkingv1.VirtualService)
	if !ok {
		return nil
	}
	return istio.MapGatewayNamesToRequest(virtualService.Spec.Gateways, virtualService.Namespace)
}

func (a *Actuator) mapIngressToGatewayRequest(_ context.Context, obj client.Object) []reconcile.Request {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil
	}
	gatewayKeys := a.state.GetGatewaysForIngress(ingress)
	if ingress.DeletionTimestamp != nil {
		a.state.RemoveIngress(ingress)
	}
	return istio.MapObjectKeysToRequests(gatewayKeys)
}

func (a *Actuator) mapServiceToGatewayRequest(_ context.Context, obj client.Object) []reconcile.Request {
	service, ok := obj.(*corev1.Service)
	if !ok {
		return nil
	}
	gatewayKeys := a.state.GetGatewaysForService(service)
	if service.DeletionTimestamp != nil {
		a.state.RemoveService(service)
	}
	return istio.MapObjectKeysToRequests(gatewayKeys)
}
