// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"net"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSOwnerType = (*api.DNSOwner)(nil)

type DNSOwnerObject struct {
	resources.Object
}

func (this *DNSOwnerObject) DNSOwner() *api.DNSOwner {
	return this.Data().(*api.DNSOwner)
}

func DNSOwner(o resources.Object) *DNSOwnerObject {
	if o.IsA(DNSOwnerType) {
		return &DNSOwnerObject{o}
	}
	return nil
}

func (this *DNSOwnerObject) Spec() *api.DNSOwnerSpec {
	return &this.DNSOwner().Spec
}

func (this *DNSOwnerObject) GetOwnerId() string {
	return this.DNSOwner().Spec.OwnerId
}

func (this *DNSOwnerObject) GetDNSActivation() *api.DNSActivation {
	return this.DNSOwner().Spec.DNSActivation
}

func (this *DNSOwnerObject) IsEnabled() bool {
	a := this.DNSOwner().Spec.Active
	return a == nil || *a
}

func (this *DNSOwnerObject) IsActive() bool {
	if this.IsEnabled() {
		valid := this.DNSOwner().Spec.ValidUntil
		if valid != nil && !valid.After(time.Now()) {
			return false
		}
		return CheckDNSActivation(this.GetCluster().GetId(), this.GetDNSActivation())
	}
	return false
}

func (this *DNSOwnerObject) ValidUntil() *metav1.Time {
	return this.DNSOwner().Spec.ValidUntil
}

func (this *DNSOwnerObject) GetCounts() map[string]int {
	return this.DNSOwner().Status.Entries.ByType
}

func (this *DNSOwnerObject) GetCount() int {
	return this.DNSOwner().Status.Entries.Amount
}

func (this *DNSOwnerObject) Status() *api.DNSOwnerStatus {
	return &this.DNSOwner().Status
}

// LookupTXTFunc is a type for looking up DNS TXT entries (or to mock it)
type LookupTXTFunc func(string) ([]string, error)

// DNSActivationLookupTXTFunc contains the actual LookupTXTFunc.
// (can be overwritten for test purposes)
var DNSActivationLookupTXTFunc LookupTXTFunc = net.LookupTXT

func CheckDNSActivation(clusterid string, activation *api.DNSActivation) bool {
	if activation == nil {
		return true
	}
	records, err := DNSActivationLookupTXTFunc(activation.DNSName)
	if err != nil {
		return false
	}
	value := clusterid
	if activation.Value != nil && *activation.Value != "" {
		value = *activation.Value
	}
	for _, r := range records {
		if r == value {
			return true
		}
	}
	return false
}
