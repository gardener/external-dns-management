// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/app/appcontext"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
	dnsanntation "github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsannotation"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	sourcednsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsprovider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/ingress"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerSwitches returns the controller switches for the DNS manager controllers.
func ControllerSwitches() *cmd.SwitchOptions {
	return cmd.NewSwitchOptions(
		cmd.Switch(dnsprovider.ControllerName, AddControlPlaneDNSProviderController),
		cmd.Switch(dnsentry.ControllerName, AddControlPlaneDNSEntryController),
		cmd.Switch(dnsanntation.ControllerName, AddSourceDNSAnnotationController),
		cmd.Switch(sourcednsprovider.ControllerName, AddSourceDNSProviderController),
		cmd.Switch(service.ControllerName, AddSourceServiceController),
		cmd.Switch(ingress.ControllerName, AddSourceIngressController),
	)
}

// AddControlPlaneDNSProviderController adds the DNSProvider control plane controller to the manager.
func AddControlPlaneDNSProviderController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&dnsprovider.Reconciler{
		DNSHandlerFactory: getStandardDNSHandlerFactory(appCtx.Config.Controllers.DNSProvider),
	}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config)
}

// AddControlPlaneDNSEntryController adds the DNSEntry control plane controller to the manager.
func AddControlPlaneDNSEntryController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&dnsentry.Reconciler{}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config)
}

// AddSourceDNSAnnotationController adds the DNSAnnotation source controller to the manager.
func AddSourceDNSAnnotationController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&dnsanntation.Reconciler{}).AddToManager(mgr, appCtx.Config)
}

// AddSourceDNSProviderController adds the DNSProvider source controller to the manager.
func AddSourceDNSProviderController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}

	if !ptr.Deref(appCtx.Config.Controllers.Source.DNSProviderReplication, false) {
		appCtx.Log.Info("DNSProvider replication is disabled")
		return nil
	}

	appCtx.Log.Info("DNSProvider replication is enabled")
	return (&sourcednsprovider.Reconciler{
		Config:            appCtx.Config.Controllers.Source,
		SourceClass:       config.GetSourceClass(appCtx.Config),
		TargetClass:       config.GetTargetClass(appCtx.Config),
		DNSHandlerFactory: getStandardDNSHandlerFactory(appCtx.Config.Controllers.DNSProvider),
	}).AddToManager(mgr, appCtx.ControlPlane)
}

// AddSourceServiceController adds the Service source controller to the manager.
func AddSourceServiceController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&service.Reconciler{
		ReconcilerBase: common.ReconcilerBase{
			Config:        appCtx.Config.Controllers.Source,
			FinalizerName: dns.ClassSourceFinalizer(dns.NormalizeClass(config.GetSourceClass(appCtx.Config)), "service-dns"),
			SourceClass:   config.GetSourceClass(appCtx.Config),
			TargetClass:   config.GetTargetClass(appCtx.Config),
			State:         state.GetState().GetAnnotationState(),
		},
	}).AddToManager(mgr, appCtx.ControlPlane)
}

// AddSourceIngressController adds the Ingress source controller to the manager.
func AddSourceIngressController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&ingress.Reconciler{
		ReconcilerBase: common.ReconcilerBase{
			Config:        appCtx.Config.Controllers.Source,
			FinalizerName: dns.ClassSourceFinalizer(dns.NormalizeClass(config.GetSourceClass(appCtx.Config)), "ingress-dns"),
			SourceClass:   config.GetSourceClass(appCtx.Config),
			TargetClass:   config.GetTargetClass(appCtx.Config),
			State:         state.GetState().GetAnnotationState(),
		},
	}).AddToManager(mgr, appCtx.ControlPlane)
}

func getStandardDNSHandlerFactory(cfg config.DNSProviderControllerConfig) provider.DNSHandlerFactory {
	s := state.GetState()
	factory := s.GetDNSHandlerFactory()
	if factory != nil {
		return factory
	}
	factory = handler.CreateStandardDNSHandlerFactory(cfg)
	return s.SetDNSHandlerFactoryOnce(factory)
}
