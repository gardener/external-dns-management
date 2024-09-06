package client

import (
	dnsmanv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	apiextensionsinstall "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/install"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	// ClusterScheme is the scheme used in garden runtime and unmanaged seed clusters.
	ClusterScheme = runtime.NewScheme()

	// ClusterSerializer is a YAML serializer using the 'ClusterScheme'.
	ClusterSerializer = json.NewSerializerWithOptions(json.DefaultMetaFactory, ClusterScheme, ClusterScheme, json.SerializerOptions{Yaml: true, Pretty: false, Strict: false})
	// ClusterCodec is a codec factory using the 'ClusterScheme'.
	ClusterCodec = serializer.NewCodecFactory(ClusterScheme)
)

func init() {
	clusterSchemeBuilder := runtime.NewSchemeBuilder(
		kubernetesscheme.AddToScheme,
		dnsmanv1alpha1.AddToScheme,
		istionetworkingv1.AddToScheme,
		istionetworkingv1alpha3.AddToScheme,
		istionetworkingv1beta1.AddToScheme,
		gatewayapisv1.AddToScheme,
		gatewayapisv1alpha2.AddToScheme,
		gatewayapisv1beta1.AddToScheme,
	)

	utilruntime.Must(clusterSchemeBuilder.AddToScheme(ClusterScheme))
	apiextensionsinstall.Install(ClusterScheme)
}
