// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"context"
	"fmt"
	"slices"
	"strings"

	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// APIVersion represents the Istio API version.
type APIVersion string

const (
	// V1Alpha3 represents Istio v1alpha3.
	V1Alpha3 APIVersion = "v1alpha3"
	// V1Beta1 represents Istio v1beta1.
	V1Beta1 APIVersion = "v1beta1"
	// V1 represents Istio v1.
	V1 APIVersion = "v1"
)

// GetDNSSpecInput constructs a DNSSpecInput from the given Istio Gateway resource.
func GetDNSSpecInput[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, state *ObjectToGatewaysState) (*common.DNSSpecInput, error) {
	annotations := common.GetMergedAnnotation(r.GVK, r.State, gatewayObj)

	annotatedNames, err := common.GetDNSNamesFromAnnotations(r.Log, annotations)
	if err != nil {
		return nil, err
	}
	if annotatedNames == nil || annotatedNames.Len() == 0 {
		return nil, nil
	}

	names, err := getDNSNames(ctx, r, gatewayObj, annotatedNames)
	if err != nil {
		return nil, err
	}
	if names == nil {
		return nil, nil
	}

	targets, err := getTargets(ctx, r, gatewayObj, annotations, annotatedNames, state)
	if err != nil {
		return nil, err
	}
	if targets == nil {
		return nil, nil
	}

	return common.AugmentFromCommonAnnotations(annotations, common.DNSSpecInput{
		Names:   names,
		Targets: targets,
	})
}

func getDNSNames[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, annotatedNames *utils.UniqueStrings) (*utils.UniqueStrings, error) {
	if annotatedNames == nil || annotatedNames.Len() == 0 {
		return nil, nil
	}

	hosts, err := extractHosts(ctx, r.Client, gatewayObj)
	if err != nil {
		return nil, err
	}

	all := annotatedNames.Contains("*")
	dnsNames := utils.NewUniqueStrings()
	for _, host := range hosts {
		if all || annotatedNames.Contains(host) {
			dnsNames.Add(host)
		}
	}

	annotatedNames.Remove("*")
	diff := annotatedNames.Difference(dnsNames)
	if len(diff) > 0 {
		return nil, fmt.Errorf("annotated dns names %s not declared by gateway.spec.servers[].hosts[]", strings.Join(diff, ", "))
	}
	return dnsNames, nil
}

func extractHosts(ctx context.Context, c client.Client, gatewayObj client.Object) ([]string, error) {
	gatewayHosts, err := getHostsFromGateway(gatewayObj)
	if err != nil {
		return nil, err
	}

	virtualServiceHosts, err := listHostsFromVirtualServices(ctx, c, gatewayObj)
	if err != nil {
		return nil, err
	}

	addHost := func(hosts []string, host string) []string {
		for _, h := range hosts {
			if h == host {
				return hosts
			}
			if strings.HasPrefix(h, "*.") && strings.HasSuffix(host, h[1:]) && !strings.Contains(host[:len(host)-len(h)+1], ".") {
				return hosts
			}
		}
		return append(hosts, host)
	}

	for _, virtualServiceHost := range virtualServiceHosts {
		gatewayHosts = addHost(gatewayHosts, virtualServiceHost)
	}
	return gatewayHosts, nil
}

func getHostsFromGateway(gatewayObj client.Object) ([]string, error) {
	hostSet := make(map[string]struct{})
	switch gateway := gatewayObj.(type) {
	case *istionetworkingv1alpha3.Gateway:
		for _, server := range gateway.Spec.Servers {
			for _, host := range server.Hosts {
				hostSet[host] = struct{}{}
			}
		}
	case *istionetworkingv1beta1.Gateway:
		for _, server := range gateway.Spec.Servers {
			for _, host := range server.Hosts {
				hostSet[host] = struct{}{}
			}
		}
	case *istionetworkingv1.Gateway:
		for _, server := range gateway.Spec.Servers {
			for _, host := range server.Hosts {
				hostSet[host] = struct{}{}
			}
		}
	default:
		return nil, fmt.Errorf("unknown gateway object: %T", gatewayObj)
	}
	hosts := make([]string, 0, len(hostSet))
	for host := range hostSet {
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func listHostsFromVirtualServices(ctx context.Context, c client.Client, gatewayObj client.Object) ([]string, error) {
	hostSet := make(map[string]struct{})
	switch gateway := gatewayObj.(type) {
	case *istionetworkingv1alpha3.Gateway:
		list := &istionetworkingv1alpha3.VirtualServiceList{}
		if err := c.List(ctx, list); err != nil {
			return nil, err
		}
		for _, virtualService := range list.Items {
			gatewayKeys := extractGatewayKeys(virtualService.Spec.Gateways, virtualService.Namespace)
			if slices.Contains(gatewayKeys, client.ObjectKeyFromObject(gateway)) {
				for _, host := range virtualService.Spec.Hosts {
					hostSet[host] = struct{}{}
				}
			}
		}
	case *istionetworkingv1beta1.Gateway:
		list := &istionetworkingv1beta1.VirtualServiceList{}
		if err := c.List(ctx, list); err != nil {
			return nil, err
		}
		for _, virtualService := range list.Items {
			gatewayKeys := extractGatewayKeys(virtualService.Spec.Gateways, virtualService.Namespace)
			if slices.Contains(gatewayKeys, client.ObjectKeyFromObject(gateway)) {
				for _, host := range virtualService.Spec.Hosts {
					hostSet[host] = struct{}{}
				}
			}
		}
	case *istionetworkingv1.Gateway:
		list := &istionetworkingv1.VirtualServiceList{}
		if err := c.List(ctx, list); err != nil {
			return nil, err
		}
		for _, virtualService := range list.Items {
			gatewayKeys := extractGatewayKeys(virtualService.Spec.Gateways, virtualService.Namespace)
			if slices.Contains(gatewayKeys, client.ObjectKeyFromObject(gateway)) {
				for _, host := range virtualService.Spec.Hosts {
					hostSet[host] = struct{}{}
				}
			}
		}
	default:
		return nil, fmt.Errorf("unknown gateway object: %T", gatewayObj)
	}
	hosts := make([]string, 0, len(hostSet))
	for host := range hostSet {
		hosts = append(hosts, host)
	}
	return hosts, nil
}

func getTargets[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, annotations map[string]string, annotatedNames *utils.UniqueStrings, state *ObjectToGatewaysState) (*utils.UniqueStrings, error) {
	if annotatedNames == nil || annotatedNames.Len() == 0 {
		return nil, nil
	}

	if targets := annotations[dns.AnnotationTargets]; targets != "" {
		return getTargetsFromAnnotation(targets), nil
	}

	if ingressName := annotations[dns.AnnotationIngress]; ingressName != "" {
		return getTargetsFromIngress(ctx, r, gatewayObj, ingressName, state)
	}

	return getTargetsFromServices(ctx, r, gatewayObj, state)
}

func getTargetsFromAnnotation(targets string) *utils.UniqueStrings {
	targetSet := utils.NewUniqueStrings()
	for target := range strings.SplitSeq(targets, ",") {
		targetSet.Add(target)
	}
	return targetSet
}

func getTargetsFromIngress[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, ingressName string, state *ObjectToGatewaysState) (*utils.UniqueStrings, error) {
	parts := strings.Split(ingressName, "/")
	var namespace, name string
	switch len(parts) {
	case 1:
		namespace = gatewayObj.GetNamespace()
		name = parts[0]
	case 2:
		namespace = parts[0]
		name = parts[1]
	default:
		return nil, fmt.Errorf("invalid annotation %s: %s", dns.AnnotationIngress, ingressName)
	}
	ingress := &networkingv1.Ingress{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ingress); err != nil {
		return nil, err
	}
	state.RemoveGatewayFromIngressMappings(gatewayObj)
	state.AddIngress(ingress, gatewayObj)
	return common.GetTargetsForIngress(ingress), nil
}

func getTargetsFromServices[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, state *ObjectToGatewaysState) (*utils.UniqueStrings, error) {
	selector, err := getSelector(gatewayObj)
	if err != nil {
		return nil, err
	}
	if len(selector) == 0 {
		return nil, nil
	}

	services := &v1.ServiceList{}
	if err := r.Client.List(ctx, services, client.MatchingLabels(selector)); err != nil {
		return nil, err
	}

	targets := utils.NewUniqueStrings()
	state.RemoveGatewayFromServiceMappings(gatewayObj)
	for _, service := range services.Items {
		serviceTargets := common.GetTargetsForService(&service, service.Annotations)
		targets.AddAll(serviceTargets.ToSlice())
		state.AddService(&service, gatewayObj)
	}
	return targets, nil
}

func getSelector(gatewayObj client.Object) (map[string]string, error) {
	switch gateway := gatewayObj.(type) {
	case *istionetworkingv1alpha3.Gateway:
		return gateway.Spec.Selector, nil
	case *istionetworkingv1beta1.Gateway:
		return gateway.Spec.Selector, nil
	case *istionetworkingv1.Gateway:
		return gateway.Spec.Selector, nil
	default:
		return nil, fmt.Errorf("unknown gateway object: %T", gatewayObj)
	}
}

// DetermineAPIVersion determines the Istio API version supported by the API server.
// It prefers API versions in the following order: v1, v1beta1, v1alpha3.
// It returns nil if no relevant CRDs are found.
func DetermineAPIVersion(dc discovery.DiscoveryInterface) (*APIVersion, error) {
	hasV1, err := hasRelevantCRDs(dc, istionetworkingv1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if hasV1 {
		return ptr.To(V1), nil // hasV1 CRDs found, no need to check for hasV1Beta1 or hasV1Alpha3
	}

	hasV1Beta1, err := hasRelevantCRDs(dc, istionetworkingv1beta1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if hasV1Beta1 {
		return ptr.To(V1Beta1), nil // hasV1Beta1 CRDs found, no need to check for hasV1Alpha3
	}

	hasV1Alpha3, err := hasRelevantCRDs(dc, istionetworkingv1alpha3.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if hasV1Alpha3 {
		return ptr.To(V1Alpha3), nil // hasV1Alpha3 CRDs found
	}

	return nil, nil // no relevant CRDs found
}

func hasRelevantCRDs(dc discovery.DiscoveryInterface, gvk schema.GroupVersion) (bool, error) {
	list, err := dc.ServerResourcesForGroupVersion(gvk.String())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, requiredKind := range []string{"Gateway", "VirtualService"} {
		if !slices.ContainsFunc(list.APIResources, func(r metav1.APIResource) bool {
			return r.Kind == requiredKind
		}) {
			return false, nil
		}
	}
	return true, nil
}

// MapGatewayNamesToRequest maps the given gateway names to reconcile.Request using the default namespace if not specified.
func MapGatewayNamesToRequest(gatewayNames []string, defaultNamespace string) []reconcile.Request {
	gatewayKeys := extractGatewayKeys(gatewayNames, defaultNamespace)
	requests := make([]reconcile.Request, len(gatewayKeys))
	for i, gatewayKey := range gatewayKeys {
		requests[i] = reconcile.Request{NamespacedName: gatewayKey}
	}
	return requests
}

func extractGatewayKeys(gatewayNames []string, defaultNamespace string) []client.ObjectKey {
	var keys []client.ObjectKey
	for _, gatewayName := range gatewayNames {
		if key := toObjectKey(gatewayName, defaultNamespace); key != nil {
			keys = append(keys, *key)
		}
	}
	return keys
}

func toObjectKey(gatewayName, defaultNamespace string) *client.ObjectKey {
	parts := strings.Split(gatewayName, "/")
	switch len(parts) {
	case 1:
		return &client.ObjectKey{Namespace: defaultNamespace, Name: parts[0]}
	case 2:
		return &client.ObjectKey{Namespace: parts[0], Name: parts[1]}
	}
	return nil
}

// MapObjectKeysToRequests maps the given []client.ObjectKey to []reconcile.Request.
func MapObjectKeysToRequests(keys []client.ObjectKey) []reconcile.Request {
	requests := make([]reconcile.Request, len(keys))
	for i, key := range keys {
		requests[i] = reconcile.Request{NamespacedName: key}
	}
	return requests
}
