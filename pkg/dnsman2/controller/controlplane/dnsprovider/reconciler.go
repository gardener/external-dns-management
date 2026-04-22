/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsprovider

import (
	"context"
	"fmt"
	"reflect"
	"time"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// Reconciler is a reconciler for DNSProvider resources on the control plane.
type Reconciler struct {
	Client            client.Client
	Clock             clock.Clock
	Recorder          events.EventRecorder
	Config            config.DNSProviderControllerConfig
	GlobalConfig      *config.DNSManagerConfiguration
	Class             string
	DNSHandlerFactory dnsprovider.DNSHandlerFactory

	state *state.State
}

// Reconcile reconciles DNSProvider resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	provider := &v1alpha1.DNSProvider{}
	if err := r.Client.Get(ctx, req.NamespacedName, provider); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	var (
		result reconcile.Result
		err    error
	)
	providerState := r.state.GetOrCreateProviderState(provider, r.Config)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, ptr.Deref(r.Config.ReconciliationTimeout, metav1.Duration{Duration: 2 * time.Minute}).Duration)
	if provider.DeletionTimestamp != nil {
		result, err = r.delete(ctxWithTimeout, log, provider)
	} else {
		result, err = r.reconcile(ctxWithTimeout, log, provider)
	}
	providerState.SetReconciled()
	if err != nil {
		log.Error(err, "reconciliation failed")
	} else if result.RequeueAfter > 0 {
		log.Info("reconciliation scheduled to be retried", "requeueAfter", result.RequeueAfter)
	} else {
		log.Info("reconciliation succeeded")
	}
	cancel()
	return result, err
}

func (r *Reconciler) addFinalizer(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) error {
	if err := controllerutils.AddFinalizers(ctx, c, provider, dns.FinalizerCompound); err != nil {
		return err
	}
	if ptr.Deref(r.Config.MigrationMode, false) {
		// In migration mode, do not add finalizers to secrets as they may be removed immediately after creation by the old controller.
		// see pkg/dns/provider/state_secret.go, method UpdateSecret() for details.
		return nil
	}
	secret, err := getSpecSecret(ctx, r.Client, provider)
	if err != nil {
		return err
	}
	if secret == nil {
		return nil
	}
	return controllerutils.AddFinalizers(ctx, c, secret, dns.FinalizerCompound)
}

func (r *Reconciler) removeFinalizer(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) error {
	if err := controllerutils.RemoveFinalizers(ctx, c, provider, dns.FinalizerCompound); err != nil {
		return err
	}
	secret, err := getSpecSecret(ctx, r.Client, provider)
	if err != nil {
		return err
	}
	if secret == nil {
		return nil
	}
	return controllerutils.RemoveFinalizers(ctx, c, secret, dns.FinalizerCompound)
}

func (r *Reconciler) updateStatusError(ctx context.Context, provider *v1alpha1.DNSProvider, err error) error {
	return r.updateStatusFailed(ctx, provider, v1alpha1.StateError, err)
}

func (r *Reconciler) updateStatusInvalid(ctx context.Context, provider *v1alpha1.DNSProvider, err error) error {
	return r.updateStatusFailed(ctx, provider, v1alpha1.StateInvalid, err)
}

func (r *Reconciler) updateStatusFailed(ctx context.Context, provider *v1alpha1.DNSProvider, state string, err error) error {
	if err := r.checkChangedSecretRef(ctx, provider); err != nil {
		return err
	}
	return r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
		status.Message = ptr.To(err.Error())
		status.State = state
		status.ObservedGeneration = provider.Generation
		status.Zones = v1alpha1.DNSSelectionStatus{}
		status.Domains = v1alpha1.DNSSelectionStatus{}
		status.RateLimit = nil
		status.DefaultTTL = nil

		// Set LastError with error codes for Gardener integration
		errorCodes := utils.DetermineErrorCodes(err)
		status.LastError = &gardencorev1beta1.LastError{
			Description: err.Error(),
			Codes:       errorCodes,
		}

		// Set LastOperation to Failed/Error state
		operationType := gardencorev1beta1.LastOperationTypeReconcile
		if provider.DeletionTimestamp != nil {
			operationType = gardencorev1beta1.LastOperationTypeDelete
		}

		operationState := gardencorev1beta1.LastOperationStateError
		// Use Failed state for non-retryable errors
		if utils.HasNonRetryableErrorCode(errorCodes) {
			operationState = gardencorev1beta1.LastOperationStateFailed
		}

		status.LastOperation = &gardencorev1beta1.LastOperation{
			Description: err.Error(),
			Progress:    0,
			State:       operationState,
			Type:        operationType,
		}

		return nil
	})
}

func (r *Reconciler) updateStatus(ctx context.Context, provider *v1alpha1.DNSProvider, modify func(status *v1alpha1.DNSProviderStatus) error) error {
	if err := r.checkChangedSecretRef(ctx, provider); err != nil {
		return err
	}

	patch := client.MergeFrom(provider.DeepCopy())
	oldStatus := provider.Status.DeepCopy()

	if err := modify(&provider.Status); err != nil {
		return err
	}
	specKey := getSpecSecretRefKey(provider)
	if specKey == nil {
		provider.Status.SecretRef = nil
	} else {
		provider.Status.SecretRef = &corev1.SecretReference{
			Name:      specKey.Name,
			Namespace: specKey.Namespace,
		}
	}
	if !equivalentStatus(oldStatus, &provider.Status) {
		timestamp := metav1.Time{Time: r.Clock.Now()}
		provider.Status.LastUpdateTime = &timestamp
	}
	if provider.Status.LastOperation != nil {
		provider.Status.LastOperation.LastUpdateTime = *provider.Status.LastUpdateTime
	}
	if provider.Status.LastError != nil {
		provider.Status.LastError.LastUpdateTime = provider.Status.LastUpdateTime
	}

	return r.Client.Status().Patch(ctx, provider, patch)
}

func (r *Reconciler) checkChangedSecretRef(ctx context.Context, provider *v1alpha1.DNSProvider) error {
	statusKey := getStatusSecretRefKey(provider)
	if statusKey == nil {
		return nil
	}
	specKey := getSpecSecretRefKey(provider)
	if specKey != nil && *statusKey == *specKey {
		return nil
	}

	secret, err := getStatusSecret(ctx, r.Client, provider)
	if err != nil {
		return err
	}
	if secret == nil {
		return nil
	}
	return controllerutils.RemoveFinalizers(ctx, r.Client, secret, dns.FinalizerCompound)
}

func getSpecSecret(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) (*corev1.Secret, error) {
	return getSecret(ctx, c, provider, provider.Spec.SecretRef, "")
}

func getStatusSecret(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider) (*corev1.Secret, error) {
	return getSecret(ctx, c, provider, provider.Status.SecretRef, "old ")
}

func getSecret(ctx context.Context, c client.Client, provider *v1alpha1.DNSProvider, secretRef *corev1.SecretReference, insert string) (*corev1.Secret, error) {
	key := getSecretRefKey(provider, secretRef)
	if key == nil {
		return nil, nil
	}

	secret := &corev1.Secret{}
	if err := c.Get(ctx, *key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("retrieving %sprovider secret failed (%s): %w", insert, *key, err)
	}
	return secret, nil
}

func getSpecSecretRefKey(provider *v1alpha1.DNSProvider) *client.ObjectKey {
	return getSecretRefKey(provider, provider.Spec.SecretRef)
}

func getStatusSecretRefKey(provider *v1alpha1.DNSProvider) *client.ObjectKey {
	return getSecretRefKey(provider, provider.Status.SecretRef)
}

func getSecretRefKey(provider *v1alpha1.DNSProvider, secretRef *corev1.SecretReference) *client.ObjectKey {
	if secretRef == nil {
		return nil
	}
	// Namespace of secret ref is silently ignored. The provider namespace is always used.
	// For compatibility with legacy dns-controller-manager SecretReferences are still necessary.
	return &client.ObjectKey{Namespace: provider.Namespace, Name: secretRef.Name}
}

func equivalentStatus(oldStatus, newStatus *v1alpha1.DNSProviderStatus) bool {
	oldStatusCopy := cleanTimeStamps(oldStatus)
	newStatusCopy := cleanTimeStamps(newStatus)
	return reflect.DeepEqual(oldStatusCopy, newStatusCopy)
}

func cleanTimeStamps(status *v1alpha1.DNSProviderStatus) *v1alpha1.DNSProviderStatus {
	statusCopy := status.DeepCopy()
	statusCopy.LastUpdateTime = nil
	if statusCopy.LastError != nil {
		statusCopy.LastError.LastUpdateTime = nil
	}
	if statusCopy.LastOperation != nil {
		statusCopy.LastOperation.LastUpdateTime = metav1.Time{}
	}
	return statusCopy
}
