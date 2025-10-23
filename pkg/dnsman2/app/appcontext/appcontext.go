// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package appcontext

import (
	"context"
	"fmt"
	"sync"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gateways_crd_watchdog"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"

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

	lock      sync.Mutex
	crdsState *gateways_crd_watchdog.CheckGatewayCRDsState
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

// GetCheckGatewayCRDsState retrieves the cached CheckGatewayCRDsState or performs the check if not cached yet.
func (ac *AppContext) GetCheckGatewayCRDsState(ctx context.Context, mgr manager.Manager) (*gateways_crd_watchdog.CheckGatewayCRDsState, error) {
	ac.lock.Lock()
	defer ac.lock.Unlock()

	if ac.crdsState != nil {
		return ac.crdsState, nil
	}

	tmpClient, err := client.New(mgr.GetConfig(), client.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return nil, err
	}
	state, err := gateways_crd_watchdog.CheckGatewayCRDs(ctx, tmpClient)
	if err != nil {
		return nil, err
	}
	ac.crdsState = state
	return state, nil
}
