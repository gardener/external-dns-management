/*
 * Copyright 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
