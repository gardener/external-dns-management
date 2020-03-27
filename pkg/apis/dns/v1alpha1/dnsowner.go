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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSOwnerList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSOwner `json:"items"`
}

// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=dnsowners,shortName=dnso,singular=dnsowner
// +kubebuilder:printcolumn:name=OwnerId,JSONPath=".spec.ownerId",type=string
// +kubebuilder:printcolumn:name=Active,JSONPath=".spec.active",type=string
// +kubebuilder:printcolumn:name=Usages,JSONPath=".status.amount",type=string
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DNSOwner struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DNSOwnerSpec   `json:"spec"`
	Status            DNSOwnerStatus `json:"status"`
}

type DNSOwnerSpec struct {
	OwnerId string `json:"ownerId,omitempty"`
	Active  *bool  `json:"active,omitempty"`
}

type DNSOwnerStatus struct {
	Entries DNSOwnerStatusEntries `json:"entries,omitempty"`
}

type DNSOwnerStatusEntries struct {
	Amount int            `json:"amount"`
	ByType map[string]int `json:"types,omitempty"`
}
