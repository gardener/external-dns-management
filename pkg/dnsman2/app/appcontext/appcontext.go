// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package appcontext

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
)

const (
	appContextName = "app-context"
)

type appContextKeyType string

var appContextKey appContextKeyType = appContextKeyType(appContextName)

// AppContext is the application context holding common objects.
type AppContext struct {
	Log          logr.Logger
	ControlPlane cluster.Cluster
	Config       *config.DNSManagerConfiguration
}

// NewAppContext creates a new application context derived from the given parent context.
func NewAppContext(parent context.Context, log logr.Logger, controlPlane cluster.Cluster, cfg *config.DNSManagerConfiguration) context.Context {
	return context.WithValue(parent, appContextKey, &AppContext{
		Log:          log,
		ControlPlane: controlPlane,
		Config:       cfg,
	})
}

// GetAppContextValue retrieves the AppContext from the given context.
func GetAppContextValue(ctx context.Context) (*AppContext, error) {
	obj := ctx.Value(appContextKey)
	if obj == nil {
		return nil, fmt.Errorf("app context not found in context")
	}
	appContext, ok := obj.(*AppContext)
	if !ok {
		return nil, fmt.Errorf("app context of invalid type %t", obj)
	}
	return appContext, nil
}
