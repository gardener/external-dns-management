// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"
	"fmt"
	"slices"
	"time"

	securityv1alpha1constants "github.com/gardener/gardener/pkg/apis/security/v1alpha1/constants"
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
	log.Info("reconcile")

	if !r.isEnabledProviderType(provider.Spec.Type) {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("provider type %q is not enabled", provider.Spec.Type))
	}

	if !r.DNSHandlerFactory.Supports(provider.Spec.Type) {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("provider type %q is not supported", provider.Spec.Type))
	}

	if err := r.addFinalizer(ctx, r.Client, provider); err != nil {
		return reconcile.Result{}, err
	}

	secretRef := r.getSecretRef(provider)
	if secretRef == nil {
		return reconcile.Result{}, r.updateStatusInvalid(ctx, provider, fmt.Errorf("no secret reference specified"))
	}
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

	providerState := r.state.GetProviderState(client.ObjectKeyFromObject(provider))
	if providerState == nil {
		return reconcile.Result{}, fmt.Errorf("internal error: provider state not found for provider %s/%s", provider.Namespace, provider.Name)
	}
	config := dnsprovider.DNSAccountConfig{
		DefaultTTL:   providerState.GetDefaultTTL(),
		ZoneCacheTTL: r.zoneCacheTTL(),
		Clock:        r.Clock,
		RateLimits:   r.Config.DefaultRateLimits,
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
		return reconcile.Result{RequeueAfter: r.recheckPeriod()}, r.updateStatusError(ctx, provider, err)
	}

	zones, err := newAccount.GetZones(ctx)
	if err != nil {
		return reconcile.Result{RequeueAfter: r.recheckPeriod()}, r.updateStatusError(ctx, provider, err)
	}
	if len(zones) == 0 {
		return reconcile.Result{RequeueAfter: r.recheckPeriod()}, r.updateStatusError(ctx, provider, fmt.Errorf("no hosted zones available in account"))
	}

	selectionResult := selection.CalcZoneAndDomainSelection(provider.Spec, toLightZones(zones))
	providerState.SetSelection(selectionResult)
	providerState.SetAccount(newAccount)

	for _, warning := range selectionResult.Warnings {
		r.Recorder.Eventf(provider, corev1.EventTypeWarning, "reconcile", "%s", warning)
	}
	result := reconcile.Result{}
	if selectionResult.Error != "" {
		result.RequeueAfter = r.recheckPeriod()
	}

	return result, r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
		if selectionResult.Error == "" {
			status.Message = ptr.To("provider operational")
			status.State = v1alpha1.StateReady
		} else {
			status.Message = ptr.To(selectionResult.Error)
			status.State = v1alpha1.StateError
		}
		status.ObservedGeneration = provider.Generation
		selectionResult.SetProviderStatusZonesAndDomains(status)
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

func (r *Reconciler) recheckPeriod() time.Duration {
	return ptr.Deref(r.Config.RecheckPeriod, metav1.Duration{Duration: 5 * time.Minute}).Duration
}

func (r *Reconciler) zoneCacheTTL() time.Duration {
	return ptr.Deref(r.Config.ZoneCacheTTL, metav1.Duration{Duration: 30 * time.Minute}).Duration
}

func (r *Reconciler) isEnabledProviderType(providerType string) bool {
	if explicitDisabled := r.Config.DisabledProviderTypes; explicitDisabled != nil {
		if slices.Contains(explicitDisabled, providerType) {
			return false
		}
	}
	if explicitEnabled := r.Config.EnabledProviderTypes; explicitEnabled != nil {
		return slices.Contains(explicitEnabled, providerType)
	}
	return true
}

func (r *Reconciler) getSecretRef(provider *v1alpha1.DNSProvider) *corev1.SecretReference {
	if provider.Spec.SecretRef == nil {
		return nil
	}
	return &corev1.SecretReference{
		Name:      provider.Spec.SecretRef.Name,
		Namespace: getSecretRefNamespace(provider),
	}
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

	props := utils.NewPropertiesFromSecretData(secret.Data)
	if err := checkAndAddWorkloadIdentitySecretLabel(secret, props); err != nil {
		return nil, err
	}
	return props, nil
}

func toLightZones(zones []dnsprovider.DNSHostedZone) []selection.LightDNSHostedZone {
	lzones := make([]selection.LightDNSHostedZone, len(zones))
	for i, z := range zones {
		lzones[i] = z
	}
	return lzones
}

func checkAndAddWorkloadIdentitySecretLabel(secret *corev1.Secret, props utils.Properties) error {
	// For security reasons, verify that the workload identity secret has expected labels,
	// but not the LabelWorkloadIdentityProvider itself as property
	if props[securityv1alpha1constants.LabelWorkloadIdentityProvider] != "" {
		return fmt.Errorf("secret %s/%s contains unexpected field %s",
			secret.GetNamespace(), secret.GetName(), securityv1alpha1constants.LabelWorkloadIdentityProvider)
	}
	if secret.Labels[securityv1alpha1constants.LabelPurpose] == securityv1alpha1constants.LabelPurposeWorkloadIdentityTokenRequestor &&
		secret.Labels[securityv1alpha1constants.LabelWorkloadIdentityProvider] != "" {
		// inject the label into the properties for further processing
		props[securityv1alpha1constants.LabelWorkloadIdentityProvider] = secret.Labels[securityv1alpha1constants.LabelWorkloadIdentityProvider]
	}
	return nil
}
