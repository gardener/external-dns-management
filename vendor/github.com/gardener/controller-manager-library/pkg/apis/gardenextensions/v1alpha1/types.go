/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// DNS is a specification for a DNS resources.
type DNS struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the DNS record resources.
	Spec DNSSpec `json:"spec"`
	// Status of the DNS record resources.
	Status DNSStatus `json:"status"`
}

// DNSSpec is the specification for a DNS resources.
type DNSSpec struct {
	// Domain is the name of the record to be registered.
	Domain string `json:"domain"`
	// HostedZoneID is the ID or name of the Hosted Zone for the given domain.
	HostedZoneID string `json:"hostedZoneID"`
	// Provider is the name of the DNS provider (e.g., aws-route53).
	Provider string `json:"provider"`
	// SecretRef is a reference to a secret that contains the credentials for the provider.
	SecretRef corev1.SecretReference `json:"secretRef"`
	// Target is the IP address or hostname to which the record shall point.
	Target string `json:"target"`
}

// DNSStatus is the status for a DNS resources.
type DNSStatus struct {
	CommonStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// DNSList is a list of DNS resources.
type DNSList struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ListMeta `json:"metadata"`
	// Items is the list of DNS.
	Items []DNS `json:"items"`
}

///////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// ControlPlane is a specification for a ControlPlane resources.
type ControlPlane struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the ControlPlane record resources.
	Spec ControlPlaneSpec `json:"spec"`
	// Status of the ControlPlane record resources.
	Status ControlPlaneStatus `json:"status"`
}

// ControlPlaneSpec is the specification for a ControlPlane resources.
type ControlPlaneSpec struct {
	CommonSpec `json:",inline"`
	// SecretRef is a reference to a secret that contains the credentials for the provider.
	SecretRef corev1.SecretReference `json:"secretRef"`
}

// ControlPlaneStatus is the status for a ControlPlane resources.
type ControlPlaneStatus struct {
	CommonStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// ControlPlaneList is a list of ControlPlane resources.
type ControlPlaneList struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ListMeta `json:"metadata"`
	// Items is the list of ControlPlane.
	Items []ControlPlane `json:"items"`
}

///////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// Infrastructure is a specification for a Infrastructure resources.
type Infrastructure struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the Infrastructure record resources.
	Spec InfrastructureSpec `json:"spec"`
	// Status of the Infrastructure record resources.
	Status InfrastructureStatus `json:"status"`
}

// InfrastructureSpec is the specification for a Infrastructure resources.
type InfrastructureSpec struct {
	CommonSpec `json:",inline"`
	// SecretRef is a reference to a secret that contains the credentials for the provider.
	SecretRef corev1.SecretReference `json:"secretRef"`
}

// InfrastructureStatus is the status for a Infrastructure resources.
type InfrastructureStatus struct {
	CommonStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.ObjectData

// InfrastructureList is a list of Infrastructure resources.
type InfrastructureList struct {
	// Standard API metadata.
	metav1.TypeMeta `json:",inline"`
	// Standard list object metadata.
	metav1.ListMeta `json:"metadata"`
	// Items is the list of Infrastructure.
	Items []Infrastructure `json:"items"`
}

///////////////////////////////////////////////////////////////////////////
///////////////////////////////////////////////////////////////////////////

type CommonSpec struct {
	OnHold         bool                  `json:"onHold,omitempty"`
	Type           string                `json:"type"`
	ProviderConfig *runtime.RawExtension `json:"providerConfig,omitempty"`
}

// CommonStatus is a struct meant to be inlined in other types. It contains common status fields.
type CommonStatus struct {
	// ObservedGeneration is the most recent generation observed for the resources. It corresponds to the
	// resources's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// State contains a base64-encoded arbitrary string. The controller must be able to restore its environment
	// out of this field.
	// +optional
	State string `json:"state,omitempty"`

	// TODO: docstring
	// +optional
	Error *Error `json:"error,omitempty"`

	// +optional
	ProviderStatus *runtime.RawExtension `json:"providerStatus,omitempty"`
}

type Error struct {
	Description    string      `json:"description"`
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
}
