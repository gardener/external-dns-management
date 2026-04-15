// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
)

// CountEntriesForProvider counts the number of DNSEntry resources currently assigned to the specified provider.
// It uses the field indexer on status.provider for efficient O(1) lookup.
// Only entries that have status.provider set are counted (i.e., entries that have been successfully provisioned).
func CountEntriesForProvider(ctx context.Context, c client.Client, namespace string, providerKey client.ObjectKey) (int32, error) {
	entryList := &v1alpha1.DNSEntryList{}
	if err := c.List(ctx, entryList,
		client.InNamespace(namespace),
		client.MatchingFields{dnsprovider.EntryStatusProvider: providerKey.String()},
	); err != nil {
		return 0, err
	}
	return int32(len(entryList.Items)), nil // #nosec G115 -- number of entries will never reach 2 billion, so int32 is sufficient
}

// quotaExceededError is returned when a provider has reached its entries quota.
type quotaExceededError struct {
	providerKey client.ObjectKey
	quota       int32
}

func (e *quotaExceededError) Error() string {
	return fmt.Sprintf("provider %s has reached its entries quota (max=%d)", e.providerKey, e.quota)
}
