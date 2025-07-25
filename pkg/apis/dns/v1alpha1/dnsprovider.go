// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSProviderList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSProvider `json:"items"`
}

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=dnsproviders,shortName=dnspr,singular=dnsprovider
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=TYPE,JSONPath=".spec.type",type=string
// +kubebuilder:printcolumn:name=STATUS,JSONPath=".status.state",type=string
// +kubebuilder:printcolumn:name=AGE,JSONPath=".metadata.creationTimestamp",type=date,description="creation timestamp"
// +kubebuilder:printcolumn:name=INCLUDED_DOMAINS,JSONPath=".status.domains.included",type=string,description="included domains"
// +kubebuilder:printcolumn:name=INCLUDED_ZONES,JSONPath=".status.zones.included",type=string,priority=2000,description="included zones"
// +kubebuilder:printcolumn:name=MESSAGE,JSONPath=".status.message",type=string,priority=2000,description="message describing the reason for the state"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DNSProviderSpec `json:"spec"`
	// +optional
	Status DNSProviderStatus `json:"status,omitempty"`
}

type DNSProviderSpec struct {
	// type of the provider (selecting the responsible type of DNS controller)
	// +kubebuilder:validation:Enum=aws-route53;alicloud-dns;azure-dns;azure-private-dns;cloudflare-dns;google-clouddns;infoblox-dns;mock-inmemory;netlify-dns;openstack-designate;powerdns;remote;rfc2136
	Type string `json:"type,omitempty"`
	// optional additional provider specific configuration values
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ProviderConfig *runtime.RawExtension `json:"providerConfig,omitempty"`
	// access credential for the external DNS system of the given type
	SecretRef *corev1.SecretReference `json:"secretRef,omitempty"`
	// desired selection of usable domains
	// (by default all zones and domains in those zones will be served)
	// +optional
	Domains *DNSSelection `json:"domains,omitempty"`
	// desired selection of usable domains
	// the domain selection is used for served zones, only
	// (by default all zones will be served)
	// +optional
	Zones *DNSSelection `json:"zones,omitempty"`
	// default TTL used for DNS entries if not specified explicitly
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=8640000
	// +optional
	DefaultTTL *int64 `json:"defaultTTL,omitempty"`
	// rate limit for create/update operations on DNSEntries assigned to this provider
	// +optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type RateLimit struct {
	// RequestsPerDay is create/update request rate per DNS entry given by requests per day
	// +kubebuilder:validation:Minimum=1
	RequestsPerDay int `json:"requestsPerDay"`
	// Burst allows bursts of up to 'burst' to exceed the rate defined by 'RequestsPerDay', while still maintaining a
	// smoothed rate of 'RequestsPerDay'
	// +kubebuilder:validation:Minimum=0
	Burst int `json:"burst"`
}

type DNSSelection struct {
	// values that should be observed (domains or zones)
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:MaxItems=100
	// + optional
	Include []string `json:"include,omitempty"`
	// values that should be ignored (domains or zones)
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:MaxItems=100
	// + optional
	Exclude []string `json:"exclude,omitempty"`
}

type DNSProviderStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// state of the provider
	// +optional
	State string `json:"state"`
	// message describing the reason for the actual state of the provider
	Message *string `json:"message,omitempty"`
	// lastUpdateTime contains the timestamp of the last status update
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	// actually served domain selection
	// +optional
	Domains DNSSelectionStatus `json:"domains"`
	// actually served zones
	// +optional
	Zones DNSSelectionStatus `json:"zones"`
	// actually used default TTL for DNS entries
	// +optional
	DefaultTTL *int64 `json:"defaultTTL,omitempty"`
	// actually used rate limit for create/update operations on DNSEntries assigned to this provider
	// +optional
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

type DNSSelectionStatus struct {
	// included values (domains or zones)
	// + optional
	Included []string `json:"included,omitempty"`
	// Excluded values (domains or zones)
	// + optional
	Excluded []string `json:"excluded,omitempty"`
}
