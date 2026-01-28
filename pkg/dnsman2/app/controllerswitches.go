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
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/crdwatch"
	sourcednsentry "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsentry"
	sourcednsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/dnsprovider"
	gatewayapiv1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1"
	gatewayapiv1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
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
		cmd.Switch(sourcednsentry.ControllerName, AddSourceDNSEntryController),
		cmd.Switch(gatewayapiv1beta1.ControllerName, AddSourceGatewayAPIV1Beta1Controller),
		cmd.Switch(gatewayapiv1.ControllerName, AddSourceGatewayAPIV1Controller),
		cmd.Switch(crdwatch.ControllerName, AddSourceCRDWatchController),
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
	return (&sourcednsprovider.Reconciler{}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config)
}

// AddSourceServiceController adds the Service source controller to the manager.
func AddSourceServiceController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return common.NewSourceReconciler(&service.Actuator{}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config, nil)
}

// AddSourceIngressController adds the Ingress source controller to the manager.
func AddSourceIngressController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return common.NewSourceReconciler(&ingress.Actuator{}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config, nil)
}

// AddSourceDNSEntryController adds the DNSEntry source controller to the manager.
func AddSourceDNSEntryController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	if mgr == appCtx.ControlPlane && dns.EquivalentClass(appCtx.Config.Class, ptr.Deref(appCtx.Config.Controllers.Source.SourceClass, "")) {
		appCtx.Log.Info("Skipping addition of DNSEntry source controller in single cluster deployment")
		return nil
	}
	return common.NewSourceReconciler(&sourcednsentry.Actuator{}).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config, nil)
}

// AddSourceGatewayAPIV1Beta1Controller adds the Gateway API v1beta1 source controller to the manager.
func AddSourceGatewayAPIV1Beta1Controller(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}

	a := &gatewayapiv1beta1.Actuator{}
	hasCRDs, err := a.HasRelevantCRDs(mgr)
	if err != nil {
		return err
	}
	if !hasCRDs {
		appCtx.Log.V(1).Info("No relevant Gateway API v1beta1 CRDs found, deactivating source controller.")
		gatewayapiv1beta1.Deactivate()
		return nil
	}

	appCtx.Log.V(1).Info("Relevant Gateway API v1beta1 CRDs found, activating source controller.")
	return common.NewSourceReconciler(a).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config, a.WatchHTTPRoutes)
}

// AddSourceGatewayAPIV1Controller adds the Gateway API v1 source controller to the manager.
func AddSourceGatewayAPIV1Controller(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}

	a := &gatewayapiv1.Actuator{}
	hasCRDs, err := a.HasRelevantCRDs(mgr)
	if err != nil {
		return err
	}
	if !hasCRDs {
		appCtx.Log.V(1).Info("No relevant Gateway API v1 CRDs found, deactivating source controller.")
		gatewayapiv1.Deactivate()
		return nil
	}

	appCtx.Log.V(1).Info("Relevant Gateway API v1 CRDs found, activating source controller.")
	return common.NewSourceReconciler(a).AddToManager(mgr, appCtx.ControlPlane, appCtx.Config, a.WatchHTTPRoutes)
}

// AddSourceCRDWatchController adds the CRD watch source controller to the manager.
func AddSourceCRDWatchController(ctx context.Context, mgr manager.Manager) error {
	appCtx, err := appcontext.GetAppContextValue(ctx)
	if err != nil {
		return err
	}
	return (&crdwatch.Reconciler{}).AddToManager(mgr, appCtx.Config)
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
