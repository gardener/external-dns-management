// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSEntryList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSEntry `json:"items"`
}

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=dnsentries,shortName=dnse,singular=dnsentry
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=DNS,description="FQDN of DNS Entry",JSONPath=".spec.dnsName",type=string
// +kubebuilder:printcolumn:name=TYPE,JSONPath=".status.providerType",type=string,description="provider type"
// +kubebuilder:printcolumn:name=PROVIDER,JSONPath=".status.provider",type=string,description="assigned provider (namespace/name)"
// +kubebuilder:printcolumn:name=STATUS,JSONPath=".status.state",type=string,description="entry status"
// +kubebuilder:printcolumn:name=AGE,JSONPath=".metadata.creationTimestamp",type=date,description="entry creation timestamp"
// +kubebuilder:printcolumn:name=TARGETS,JSONPath=".status.targets",type=string,description="effective targets"
// +kubebuilder:printcolumn:name=TTL,JSONPath=".status.ttl",type=integer,priority=2000,description="time to live"
// +kubebuilder:printcolumn:name=ZONE,JSONPath=".status.zone",type=string,priority=2000,description="zone id"
// +kubebuilder:printcolumn:name=POLICY_TYPE,JSONPath=".status.routingPolicy.type",type=string,priority=2000,description="routing policy type"
// +kubebuilder:printcolumn:name=POLICY_SETID,JSONPath=".status.routingPolicy.setIdentifier",type=string,priority=2000,description="routing policy set identifier"
// +kubebuilder:printcolumn:name=POLICY_PARAMS,JSONPath=".status.routingPolicy.parameters",type=string,priority=2000,description="routing policy parameters"
// +kubebuilder:printcolumn:name=MESSAGE,JSONPath=".status.message",type=string,priority=2000,description="message describing the reason for the state"
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DNSEntrySpec `json:"spec"`
	// +optional
	Status DNSEntryStatus `json:"status,omitempty"`
}

type DNSEntrySpec struct {
	// full qualified domain name
	DNSName string `json:"dnsName"`
	// reference to base entry used to inherit attributes from
	// +optional
	Reference *EntryReference `json:"reference,omitempty"`
	// Deprecated: This field is no longer used and will be removed in a future release.
	// +kubebuilder:validation:Deprecated
	// +optional
	OwnerId *string `json:"ownerId,omitempty"`
	// time to live for records in external DNS system
	// +optional
	TTL *int64 `json:"ttl,omitempty"`
	// lookup interval for CNAMEs that must be resolved to IP addresses.
	// Only used if `resolveTargetsToAddresses` is set to true or targets consists of multiple domain names.
	// +optional
	CNameLookupInterval *int64 `json:"cnameLookupInterval,omitempty"`
	// enables translation of a target domain name in the resolved IPv4 and IPv6 addresses.
	// If enabled, `A` and/or `AAAA` records are created instead of a `CNAME` record.
	// If the target list contains multiple targets, it is enabled implicitly.
	// +optional
	ResolveTargetsToAddresses *bool `json:"resolveTargetsToAddresses,omitempty"`
	// text records, either text or targets must be specified
	// +optional
	Text []string `json:"text,omitempty"`
	// target records (CNAME or A records), either text or targets must be specified
	// +optional
	Targets []string `json:"targets,omitempty"`
	// optional routing policy
	// +optional
	RoutingPolicy *RoutingPolicy `json:"routingPolicy,omitempty"`
}

type DNSEntryStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// entry state
	// +optional
	State string `json:"state"`
	// message describing the reason for the state
	// +optional
	Message *string `json:"message,omitempty"`
	// lastUpdateTime contains the timestamp of the last status update
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	// provider type used for the entry
	// +optional
	ProviderType *string `json:"providerType,omitempty"`
	// assigned provider
	// +optional
	Provider *string `json:"provider,omitempty"`
	// zone used for the entry
	// +optional
	Zone *string `json:"zone,omitempty"`
	// time to live used for the entry
	// +optional
	TTL *int64 `json:"ttl,omitempty"`
	// effective targets generated for the entry
	// +optional
	Targets []string `json:"targets,omitempty"`
	// effective routing policy
	// +optional
	RoutingPolicy *RoutingPolicy `json:"routingPolicy,omitempty"`
	// effective lookup interval for CNAMEs that must be resolved to IP addresses
	// +optional
	CNameLookupInterval *int64 `json:"cnameLookupInterval,omitempty"`
}

type EntryReference struct {
	// name of the referenced DNSEntry object
	Name string `json:"name"`
	// namespace of the referenced DNSEntry object
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type RoutingPolicy struct {
	// Policy is the policy type. Allowed values are provider dependent, e.g. `weighted`
	Type string `json:"type"`
	// SetIdentifier is the identifier of the record set
	SetIdentifier string `json:"setIdentifier"`
	// Policy specific parameters
	Parameters map[string]string `json:"parameters"`
}
