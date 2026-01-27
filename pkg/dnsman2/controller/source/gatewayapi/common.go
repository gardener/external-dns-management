// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// GetGVKV1beta1 returns the GroupVersionKind for Gateway API v1beta1 Gateway resource.
func GetGVKV1beta1() schema.GroupVersionKind {
	resource := gatewayapisv1beta1.Resource("Gateway").WithVersion("v1beta1")
	return schema.GroupVersionKind{
		Group:   resource.Group,
		Version: resource.Version,
		Kind:    resource.Resource,
	}
}

// GetGVKV1 returns the GroupVersionKind for Gateway API v1 Gateway resource.
func GetGVKV1() schema.GroupVersionKind {
	resource := gatewayapisv1beta1.Resource("Gateway").WithVersion("v1")
	return schema.GroupVersionKind{
		Group:   resource.Group,
		Version: resource.Version,
		Kind:    resource.Resource,
	}
}

// GetDNSSpecInput constructs a DNSSpecInput from the given Gateway resource.
func GetDNSSpecInput[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object) (*common.DNSSpecInput, error) {
	annotations := common.GetMergedAnnotation(r.GVK, r.State, gatewayObj)

	names, err := getDNSNames(ctx, r, gatewayObj, annotations)
	if err != nil {
		return nil, err
	}
	if names == nil {
		return nil, nil
	}

	targets, err := getTargets(gatewayObj)
	if err != nil {
		return nil, err
	}

	return common.AugmentFromCommonAnnotations(annotations, common.DNSSpecInput{
		Names:   names,
		Targets: targets,
	})
}

// ExtractGatewayKeys extracts the gateway keys from the given HTTPRoute resource
func ExtractGatewayKeys(gvk schema.GroupVersionKind, route *gatewayapisv1.HTTPRoute) []client.ObjectKey {
	var keys []client.ObjectKey
	for _, ref := range route.Spec.ParentRefs {
		if (ref.Group == nil || string(*ref.Group) == gvk.GroupVersion().String()) &&
			(ref.Kind == nil || string(*ref.Kind) == gvk.Kind) {
			namespace := route.Namespace
			if ref.Namespace != nil {
				namespace = string(*ref.Namespace)
			}
			keys = append(keys, client.ObjectKey{Namespace: namespace, Name: string(ref.Name)})
		}
	}
	return keys
}

// HasRelevantCRDs checks whether the required Gateway API CRDs are present in the cluster.
func HasRelevantCRDs(mgr manager.Manager, gvk schema.GroupVersionKind) (bool, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return false, err
	}
	resources, err := dc.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return false, err
	}

	kinds := make(map[string]struct{})
	for _, resource := range resources.APIResources {
		kinds[resource.Kind] = struct{}{}
	}

	if _, found := kinds["Gateway"]; !found {
		return false, nil
	}
	if _, found := kinds["HTTPRoute"]; !found {
		return false, nil
	}
	return true, nil
}

func getDNSNames[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object, annotations map[string]string) (*utils.UniqueStrings, error) {
	annotatedNames, err := common.GetDNSNamesFromAnnotations(r.Log, annotations)
	if err != nil {
		return nil, err
	}
	if annotatedNames == nil {
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
		return nil, fmt.Errorf("annotated dns names %s not declared by gateway.spec.listeners[].hostname", strings.Join(diff, ", "))
	}
	return dnsNames, nil
}

func extractHosts(ctx context.Context, c client.Client, gatewayObj client.Object) ([]string, error) {
	listeners, err := getListeners(gatewayObj)
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, listener := range listeners {
		if listener.Hostname != nil {
			hosts = append(hosts, string(*listener.Hostname))
		}
	}

	routes, err := listHTTPRoutes(ctx, c, gatewayObj)
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

	for _, route := range routes {
		for _, host := range route.Spec.Hostnames {
			hosts = addHost(hosts, string(host))
		}
	}
	return hosts, nil
}

func getListeners(gatewayObj client.Object) ([]gatewayapisv1.Listener, error) {
	switch gateway := gatewayObj.(type) {
	case *gatewayapisv1beta1.Gateway:
		return gateway.Spec.Listeners, nil
	case *gatewayapisv1.Gateway:
		return gateway.Spec.Listeners, nil
	default:
		return nil, fmt.Errorf("unknown gateway object: %T", gateway)
	}
}

func listHTTPRoutes(ctx context.Context, c client.Client, gatewayObj client.Object) ([]gatewayapisv1.HTTPRoute, error) {
	routes, gvk, err := listHTTPRoutesFor(ctx, c, gatewayObj)
	if err != nil {
		return nil, err
	}

	var gatewayRoutes []gatewayapisv1.HTTPRoute
	for _, route := range routes {
		gatewayKeys := ExtractGatewayKeys(*gvk, &route)
		for _, gatewayKey := range gatewayKeys {
			if gatewayKey == client.ObjectKeyFromObject(gatewayObj) {
				gatewayRoutes = append(gatewayRoutes, route)
			}
		}
	}
	return gatewayRoutes, nil
}

func listHTTPRoutesFor(ctx context.Context, c client.Client, gatewayObj client.Object) ([]gatewayapisv1.HTTPRoute, *schema.GroupVersionKind, error) {
	switch gateway := gatewayObj.(type) {
	case *gatewayapisv1beta1.Gateway:
		list := &gatewayapisv1beta1.HTTPRouteList{}
		err := c.List(ctx, list)
		if err != nil {
			return nil, nil, err
		}
		routes := make([]gatewayapisv1.HTTPRoute, len(list.Items))
		for i, route := range list.Items {
			routes[i] = gatewayapisv1.HTTPRoute(route)
		}
		return routes, ptr.To(GetGVKV1beta1()), nil
	case *gatewayapisv1.Gateway:
		list := &gatewayapisv1.HTTPRouteList{}
		err := c.List(ctx, list)
		if err != nil {
			return nil, nil, err
		}
		return list.Items, ptr.To(GetGVKV1()), nil
	default:
		return nil, nil, fmt.Errorf("unknown gateway object: %T", gateway)
	}
}

func getTargets(gatewayObj client.Object) (*utils.UniqueStrings, error) {
	addresses, err := getStatusAddresses(gatewayObj)
	if err != nil {
		return nil, err
	}
	ips := utils.NewUniqueStrings()
	hosts := utils.NewUniqueStrings()
	for _, address := range addresses {
		switch *address.Type {
		case gatewayapisv1.IPAddressType:
			ips.Add(address.Value)
		case gatewayapisv1.HostnameAddressType:
			hosts.Add(address.Value)
		}
	}
	if ips.Len() > 0 {
		return ips, nil
	}
	return hosts, nil
}

func getStatusAddresses(gatewayObj client.Object) ([]gatewayapisv1.GatewayStatusAddress, error) {
	switch gateway := gatewayObj.(type) {
	case *gatewayapisv1beta1.Gateway:
		return gateway.Status.Addresses, nil
	case *gatewayapisv1.Gateway:
		return gateway.Status.Addresses, nil
	default:
		return nil, fmt.Errorf("unknown gateway object: %T", gateway)
	}
}
