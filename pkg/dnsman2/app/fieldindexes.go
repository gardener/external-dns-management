// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
)

// AddAllFieldIndexesToManager adds all field indices to the given manager.
func AddAllFieldIndexesToManager(ctx context.Context, mgr manager.Manager) error {
	for _, fn := range []func(context.Context, client.FieldIndexer) error{
		dnsprovider.AddEntryStatusProvider,
	} {
		if err := fn(ctx, mgr.GetFieldIndexer()); err != nil {
			return err
		}
	}

	return nil
}
