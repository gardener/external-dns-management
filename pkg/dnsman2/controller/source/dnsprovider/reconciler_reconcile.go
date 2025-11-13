// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, sourceProvider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	log.Info("reconcile")

	if err := addFinalizer(ctx, r.Client, sourceProvider); err != nil {
		return reconcile.Result{}, err
	}

	targetProviders, err := r.getExistingTargetProviders(ctx, sourceProvider)
	if err != nil {
		return reconcile.Result{}, err
	}

	var newProvider *v1alpha1.DNSProvider
	for _, targetProvider := range targetProviders {
		if targetProvider.Spec.Type == sourceProvider.Spec.Type {
			newProvider = &targetProvider
			break
		}
	}
	if newProvider == nil {
		newProvider = &v1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: r.generateNameTemplate(sourceProvider),
				Namespace:    r.targetNamespace(sourceProvider),
			},
		}
	}

	if err := r.deleteObsoleteTargetProviders(ctx, log, sourceProvider, targetProviders, newProvider); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.createOrUpdateTargetProvider(ctx, log, sourceProvider, newProvider); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) generateNameTemplate(sourceProvider *v1alpha1.DNSProvider) string {
	return strings.ToLower(fmt.Sprintf("%s%s-", ptr.Deref(r.Config.Controllers.Source.TargetNamePrefix, ""), sourceProvider.GetName()))
}

func (r *Reconciler) getExistingTargetProviders(ctx context.Context, sourceProvider *v1alpha1.DNSProvider) ([]v1alpha1.DNSProvider, error) {
	candidates := &v1alpha1.DNSProviderList{}
	if err := r.ControlPlaneClient.List(ctx, candidates, client.InNamespace(r.targetNamespace(sourceProvider))); err != nil {
		return nil, fmt.Errorf("failed to list target DNSProviders: %w", err)
	}

	var targetProviders []v1alpha1.DNSProvider
	for _, candidate := range candidates.Items {
		if r.isOwnedByController(&candidate, sourceProvider) {
			targetProviders = append(targetProviders, candidate)
		}
	}
	return targetProviders, nil
}

func (r *Reconciler) deleteObsoleteTargetProviders(
	ctx context.Context,
	log logr.Logger,
	sourceProvider *v1alpha1.DNSProvider,
	existingTargetProviders []v1alpha1.DNSProvider,
	providerToKeep *v1alpha1.DNSProvider,
) error {
	for _, targetProvider := range existingTargetProviders {
		if providerToKeep != nil && targetProvider.Name == providerToKeep.Name {
			continue
		}
		if err := r.ControlPlaneClient.Delete(ctx, &targetProvider); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete obsolete target DNSProvider %s: %w", client.ObjectKeyFromObject(&targetProvider), err)
		}
		log.Info("deleted obsolete target DNSProvider", "name", targetProvider.Name)
		r.Recorder.Eventf(sourceProvider, corev1.EventTypeNormal, "DNSProviderDeleted", "Deleted DNSProvider: %s", targetProvider.Name) // TODO: check former reason/message
	}
	return nil
}

func (r *Reconciler) createOrUpdateTargetProvider(
	ctx context.Context,
	log logr.Logger,
	sourceProvider *v1alpha1.DNSProvider,
	targetProvider *v1alpha1.DNSProvider,
) error {
	targetSecret, err := r.createOrUpdateTargetSecretFromSourceSecret(ctx, sourceProvider, targetProvider)
	if err != nil {
		return err
	}

	opResult, err := controllerutil.CreateOrUpdate(ctx, r.ControlPlaneClient, targetProvider, func() error {
		targetProvider.Spec = *sourceProvider.Spec.DeepCopy()
		if targetSecret != nil {
			targetProvider.Spec.SecretRef = &corev1.SecretReference{Name: targetSecret.Name, Namespace: targetSecret.Namespace}
		} else {
			targetProvider.Spec.SecretRef = nil
		}
		if targetProvider.Annotations == nil {
			targetProvider.Annotations = make(map[string]string)
		}
		if r.Config.Controllers.Source.TargetLabels != nil {
			for key, value := range r.Config.Controllers.Source.TargetLabels {
				utils.SetLabel(targetProvider, key, value)
			}
		}
		r.buildOwnerData(sourceProvider).AddOwner(targetProvider, ptr.Deref(r.Config.Controllers.Source.TargetClusterID, ""))
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create or update target DNSProvider %s: %w", client.ObjectKeyFromObject(targetProvider), err)
	}
	switch opResult {
	case controllerutil.OperationResultCreated:
		log.Info("created target DNSProvider", "name", targetProvider.Name)
		r.Recorder.Eventf(sourceProvider, corev1.EventTypeNormal, "DNSProviderCreated", "Created DNSProvider: %s", targetProvider.Name)
	case controllerutil.OperationResultUpdated:
		log.Info("updated target DNSProvider", "name", targetProvider.Name)
		r.Recorder.Eventf(sourceProvider, corev1.EventTypeNormal, "DNSProviderUpdated", "Updated DNSProvider: %s", targetProvider.Name)
	case controllerutil.OperationResultNone:
		// no-op
	}

	if targetSecret == nil {
		if sourceProvider.Spec.SecretRef == nil {
			return r.updateStatusInvalid(ctx, sourceProvider, "secretRef not set")
		}
		return r.updateStatusInvalid(ctx, sourceProvider, fmt.Sprintf("secret %s/%s not found", getSecretRefNamespace(sourceProvider), sourceProvider.Spec.SecretRef.Name))
	}

	if err := r.ensureOwnerReferenceOnSecret(ctx, targetSecret, targetProvider); err != nil {
		return err
	}

	if validationErrorMsg := targetSecret.Annotations[dns.AnnotationValidationError]; validationErrorMsg != "" {
		return r.updateStatusInvalid(ctx, sourceProvider, validationErrorMsg)
	}

	return r.updateStatus(ctx, sourceProvider, func(status *v1alpha1.DNSProviderStatus) error {
		status.Message = targetProvider.Status.Message
		status.State = targetProvider.Status.State
		status.Domains = targetProvider.Status.Domains
		status.Zones = targetProvider.Status.Zones
		status.RateLimit = targetProvider.Status.RateLimit
		status.DefaultTTL = targetProvider.Status.DefaultTTL
		status.LastUpdateTime = targetProvider.Status.LastUpdateTime
		status.ObservedGeneration = targetProvider.Generation
		return nil
	})
}

func (r *Reconciler) ensureOwnerReferenceOnSecret(ctx context.Context, targetSecret *corev1.Secret, targetProvider *v1alpha1.DNSProvider) error {
	patch := client.MergeFrom(targetSecret.DeepCopy())
	if err := controllerutil.SetOwnerReference(targetProvider, targetSecret, r.ControlPlaneClient.Scheme()); err != nil {
		return fmt.Errorf("failed to set owner reference on secret %s: %w", client.ObjectKeyFromObject(targetSecret), err)
	}
	if err := r.ControlPlaneClient.Patch(ctx, targetSecret, patch); err != nil {
		return fmt.Errorf("failed to patch owner reference on secret %s: %w", client.ObjectKeyFromObject(targetSecret), err)
	}
	return nil
}

func (r *Reconciler) createOrUpdateTargetSecretFromSourceSecret(
	ctx context.Context,
	sourceProvider *v1alpha1.DNSProvider,
	targetProvider *v1alpha1.DNSProvider,
) (*corev1.Secret, error) {
	secretRef := targetProvider.Spec.SecretRef
	if sourceProvider.Spec.SecretRef == nil {
		return nil, nil
	}
	sourceSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceProvider.Spec.SecretRef.Name,
			Namespace: getSecretRefNamespace(sourceProvider),
		},
	}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(sourceSecret), sourceSecret); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get secret %s for source DNSProvider %s: %w", client.ObjectKeyFromObject(sourceSecret), client.ObjectKeyFromObject(sourceProvider), err)
	}

	props := utils.NewPropertiesFromSecretData(sourceSecret.Data)
	var annotations map[string]string
	adapter, validationErr := r.DNSHandlerFactory.GetDNSHandlerAdapter(sourceProvider.Spec.Type)
	if validationErr == nil {
		validationErr = adapter.ValidateCredentialsAndProviderConfig(props, sourceProvider.Spec.ProviderConfig)
	}
	if validationErr != nil {
		// If validation fails, we store the error in the secret annotations.
		// The annotations will be used to fill the status message of the replicated provider and will
		// be pushed back to the source provider.
		annotations = map[string]string{dns.AnnotationValidationError: validationErr.Error()}
		sourceSecret.Data = nil // remove data if validation fails
	}

	modify := func(secret *corev1.Secret) {
		secret.Data = sourceSecret.Data
		secret.Type = sourceSecret.Type
		secret.Annotations = annotations
	}

	if secretRef == nil {
		targetSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: r.generateNameTemplate(sourceProvider),
				Namespace:    r.targetNamespace(targetProvider),
			},
		}
		modify(targetSecret)
		if err := r.ControlPlaneClient.Create(ctx, targetSecret); err != nil {
			return nil, fmt.Errorf("failed to create secret for target DNSProvider %s: %w", client.ObjectKeyFromObject(targetProvider), err)
		}
		return targetSecret, nil
	}

	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetProvider.Spec.SecretRef.Name,
			Namespace: getSecretRefNamespace(targetProvider),
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.ControlPlaneClient, targetSecret, func() error {
		modify(targetSecret)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create or update secret %s for target DNSProvider %s: %w", client.ObjectKeyFromObject(targetSecret), client.ObjectKeyFromObject(targetProvider), err)
	}
	return targetSecret, nil
}

func (r *Reconciler) targetNamespace(owner metav1.Object) string {
	return ptr.Deref(r.Config.Controllers.Source.TargetNamespace, owner.GetNamespace())
}

func (r *Reconciler) isOwnedByController(target, owner *v1alpha1.DNSProvider) bool {
	return r.buildOwnerData(owner).HasOwner(target, ptr.Deref(r.Config.Controllers.Source.TargetClusterID, ""))
}

func (r *Reconciler) buildOwnerData(owner *v1alpha1.DNSProvider) common.OwnerData {
	return common.OwnerData{
		Object:    owner,
		ClusterID: ptr.Deref(r.Config.Controllers.Source.SourceClusterID, ""),
		GVK:       r.GVK,
	}
}
