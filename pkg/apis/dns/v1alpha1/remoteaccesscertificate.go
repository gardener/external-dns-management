// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RemoteAccessCertificateList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteAccessCertificate `json:"items"`
}

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,path=remoteaccesscertificates,shortName=remotecert,singular=remoteaccesscertificate
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=Type,JSONPath=".spec.type",type=string
// +kubebuilder:printcolumn:name=Age,JSONPath=".metadata.creationTimestamp",type=date
// +kubebuilder:printcolumn:name=SecretAge,JSONPath=".status.notBefore",type=date
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RemoteAccessCertificate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RemoteAccessCertificateSpec `json:"spec"`
	// +optional
	Status RemoteAccessCertificateStatus `json:"status,omitempty"`
}

type RemoteAccessCertificateSpec struct {
	// Certificate type (client or server)
	Type RemoteAccessCertificateType `json:"type"`
	// Name of the secret to store the client certificate
	SecretName string `json:"secretName"`
	// Domain name, used for building subject and DNS name
	DomainName string `json:"domainName"`
	// Number of days the certificate should be valid
	Days int `json:"days"`
	// Indicates if certificate should be recreated and replaced in the secret
	// +optional
	Recreate bool `json:"recreate,omitempty"`
}

// RemoteAccessCertificateType is a string alias.
type RemoteAccessCertificateType string

const (
	// ServerType specifies that the certificate is a server certificate
	ServerType RemoteAccessCertificateType = "server"
	// ClientType specifies that the certificate is a client certificate
	ClientType RemoteAccessCertificateType = "client"
)

type RemoteAccessCertificateStatus struct {
	// Creation timestamp of the certificate
	// +optional
	NotBefore *metav1.Time `json:"notBefore,omitempty"`
	// Expiration timestamp of the certificate
	// +optional
	NotAfter *metav1.Time `json:"notAfter,omitempty"`
	// Serial number of the certificate
	// +optional
	SerialNumber *string `json:"serialNumber,omitempty"`
	// In case of a configuration problem this field describes the reason
	// +optional
	Message string `json:"message,omitempty"`
	// Indicates if certificate should be recreated and replaced in the secret
	// +optional
	Recreating bool `json:"recreating,omitempty"`
}
