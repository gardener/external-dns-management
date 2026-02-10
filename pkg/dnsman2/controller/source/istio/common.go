// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"context"
	"slices"

	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
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
func GetDNSSpecInput[T client.Object](ctx context.Context, r *common.SourceReconciler[T], gatewayObj client.Object) (*common.DNSSpecInput, error) {
	annotations := common.GetMergedAnnotation(r.GVK, r.State, gatewayObj)

	// TODO
	//names, err := getDNSNames(ctx, r, gatewayObj, annotations)
	//if err != nil {
	//	return nil, err
	//}
	//if names == nil {
	//	return nil, nil
	//}
	//
	//targets, err := getTargets(gatewayObj)
	//if err != nil {
	//	return nil, err
	//}

	return common.AugmentFromCommonAnnotations(annotations, common.DNSSpecInput{
		//Names:   names,
		//Targets: targets,
	})
}

// DetermineAPIVersion determines the Istio API version supported by the API server.
// It prefers API versions in the following order: v1, v1beta1, v1alpha3.
// It returns nil if no relevant CRDs are found.
func DetermineAPIVersion(dc discovery.DiscoveryInterface) (*APIVersion, error) {
	v1, err := hasRelevantCRDs(dc, istionetworkingv1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if v1 {
		return ptr.To(V1), nil // v1 CRDs found, no need to check for v1beta1 or v1alpha3
	}

	v1beta1, err := hasRelevantCRDs(dc, istionetworkingv1beta1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if v1beta1 {
		return ptr.To(V1Beta1), nil // v1beta1 CRDs found, no need to check for v1alpha3
	}

	v1alpha3, err := hasRelevantCRDs(dc, istionetworkingv1alpha3.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	if v1alpha3 {
		return ptr.To(V1Alpha3), nil // v1alpha3 CRDs found
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
