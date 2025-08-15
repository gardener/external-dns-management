// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSAnnotationList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSAnnotation `json:"items"`
}

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=dnsannotations,shortName=dnsa,singular=dnsannotation
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=RefGroup,JSONPath=".spec.resourceRef.apiVersion",type=string
// +kubebuilder:printcolumn:name=RefKind,JSONPath=".spec.resourceRef.kind",type=string
// +kubebuilder:printcolumn:name=RefName,JSONPath=".spec.resourceRef.name",type=string
// +kubebuilder:printcolumn:name=RefNamespace,JSONPath=".spec.resourceRef.namespace",type=string
// +kubebuilder:printcolumn:name=Active,JSONPath=".status.active",type=boolean
// +kubebuilder:printcolumn:name=Age,JSONPath=".metadata.creationTimestamp",type=date
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSAnnotation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DNSAnnotationSpec `json:"spec"`
	// +optional
	Status DNSAnnotationStatus `json:"status,omitempty"`
}

type DNSAnnotationSpec struct {
	// ResourceRef specifies the resource that should be annotated.
	ResourceRef ResourceReference `json:"resourceRef"`
	// Annotations to be added to the referenced resource.
	// +kubebuilder:validation:MinProperties=1
	// +kubebuilder:validation:MaxProperties=20
	Annotations map[string]string `json:"annotations"`
}

type ResourceReference struct {
	// API Version of the annotated object
	// +kubebuilder:validation:Enum=v1;networking.k8s.io/v1;gateway.networking.k8s.io/v1;gateway.networking.k8s.io/v1beta1;networking.istio.io/v1;networking.istio.io/v1beta1;networking.istio.io/v1alpha3
	APIVersion string `json:"apiVersion"`
	// Kind of the annotated object
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +kubebuilder:validation:Enum=Service;Ingress;Gateway
	Kind string `json:"kind"`
	// Name of the annotated object
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name,omitempty"`
	// Namespace of the annotated object
	// Defaulted by the namespace of the containing resource.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Namespace string `json:"namespace,omitempty"`
}

type DNSAnnotationStatus struct {
	// Indicates that annotation is observed by a DNS source controller
	// +optional
	Active bool `json:"active,omitempty"`
	// In case of a configuration problem this field describes the reason
	// +optional
	Message string `json:"message,omitempty"`
}
