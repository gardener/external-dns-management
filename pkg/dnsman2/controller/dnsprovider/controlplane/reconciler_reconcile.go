// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	config2 "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, provider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	if !r.isEnabledProviderType(provider.Spec.Type) {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("provider type %q is not enabled", provider.Spec.Type))
	}

	if !r.DNSHandlerFactory.Supports(provider.Spec.Type) {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("provider type %q is not supported", provider.Spec.Type))
	}

	if err := addFinalizer(ctx, r.Client, provider); err != nil {
		return reconcile.Result{}, err
	}

	secretRef := r.getSecretRef(provider)
	if secretRef == nil {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("no secret reference specified"))
	}
	providerState := r.state.GetOrCreateProviderState(provider, r.Config.Controllers.DNSProvider)
	props, err := r.getProperties(ctx, secretRef)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.updateStatusError(ctx, provider, fmt.Errorf("secret %s/%s not found", secretRef.Namespace, secretRef.Name))
		}
		return reconcile.Result{}, err
	}

	adapter, err := r.state.GetDNSHandlerFactory().GetDNSHandlerAdapter(provider.Spec.Type)
	if err != nil {
		return reconcile.Result{}, err
	}
	if err := adapter.ValidateCredentialsAndProviderConfig(props, provider.Spec.ProviderConfig); err != nil {
		return reconcile.Result{}, r.updateStatusError(ctx, provider, fmt.Errorf("secret %s/%s validation failed: %s", secretRef.Namespace, secretRef.Name, err))
	}

	config := dnsprovider.DNSAccountConfig{
		DefaultTTL:   providerState.GetDefaultTTL(),
		ZoneCacheTTL: ptr.Deref(r.Config.Controllers.DNSProvider.ZoneCacheTTL, metav1.Duration{Duration: 5 * time.Minute}).Duration,
		Clock:        r.Clock,
		RateLimits:   r.Config.Controllers.DNSProvider.DefaultRateLimits,
		Factory:      r.DNSHandlerFactory,
	}
	if provider.Spec.RateLimit != nil {
		config.RateLimits = &config2.RateLimiterOptions{
			Enabled: true,
			QPS:     float32(1.0 * provider.Spec.RateLimit.RequestsPerDay / (60 * 60 * 24)),
			Burst:   provider.Spec.RateLimit.Burst,
		}
	}
	newAccount, err := r.state.GetAccount(log, provider, props, config)
	if err != nil {
		return reconcile.Result{}, err
	}

	zones, err := newAccount.GetZones(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if len(zones) == 0 {
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, r.updateStatusError(ctx, provider, fmt.Errorf("no hosted zones available in account"))
	}

	providerState.SetSelection(selection.CalcZoneAndDomainSelection(provider.Spec, toLightZones(zones)))

	providerState.SetAccount(newAccount)

	return reconcile.Result{}, r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
		status.Message = nil
		status.State = v1alpha1.StateReady
		status.ObservedGeneration = provider.Generation
		providerState.GetSelection().SetProviderStatusZonesAndDomains(status)
		status.DefaultTTL = ptr.To[int64](providerState.GetDefaultTTL())
		if config.RateLimits != nil && config.RateLimits.Enabled {
			status.RateLimit = &v1alpha1.RateLimit{
				RequestsPerDay: int(config.RateLimits.QPS * 60 * 60 * 24),
				Burst:          config.RateLimits.Burst,
			}
		} else {
			status.RateLimit = nil
		}
		return nil
	})
}

func (r *Reconciler) isEnabledProviderType(providerType string) bool {
	if explicitDisabled := r.Config.Controllers.DNSProvider.DisabledProviderTypes; explicitDisabled != nil {
		for _, disabledType := range explicitDisabled {
			if providerType == disabledType {
				return false
			}
		}
	}
	if explicitEnabled := r.Config.Controllers.DNSProvider.EnabledProviderTypes; explicitEnabled != nil {
		for _, enabledType := range explicitEnabled {
			if providerType == enabledType {
				return true
			}
		}
		return false
	}
	return true
}

func (r *Reconciler) getSecretRef(provider *v1alpha1.DNSProvider) *corev1.SecretReference {
	if provider.Spec.SecretRef == nil {
		return nil
	}
	ref := &corev1.SecretReference{
		Name:      provider.Spec.SecretRef.Name,
		Namespace: provider.Spec.SecretRef.Namespace,
	}
	if ref.Namespace == "" {
		ref.Namespace = provider.Namespace
	}
	return ref
}

func (r *Reconciler) getProperties(ctx context.Context, secretRef *corev1.SecretReference) (utils.Properties, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRef.Name,
			Namespace: secretRef.Namespace,
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return nil, fmt.Errorf("unable to get secret %s: %w", client.ObjectKeyFromObject(secret), err)
	}

	return toProperties(secret.Data), nil
}

func toProperties(data map[string][]byte) utils.Properties {
	props := utils.Properties{}
	for k, v := range data {
		props[k] = string(v)
	}
	return props
}

func toLightZones(zones []dnsprovider.DNSHostedZone) []selection.LightDNSHostedZone {
	lzones := make([]selection.LightDNSHostedZone, len(zones))
	for i, z := range zones {
		lzones[i] = z
	}
	return lzones
}
