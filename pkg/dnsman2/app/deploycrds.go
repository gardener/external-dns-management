// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"k8s.io/utils/set"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apidns "github.com/gardener/external-dns-management/pkg/apis/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DeployCRDs deploys the required CRDs for the DNSManager if/as specified in the given configuration.
func DeployCRDs(ctx context.Context, log logr.Logger, restConfig *rest.Config, cfg *config.DNSManagerConfiguration) error {
	c, err := client.New(restConfig, client.Options{
		Scheme: dnsmanclient.ClusterScheme,
	})
	if err != nil {
		return fmt.Errorf("DeployCRDs: could not create client: %w", err)
	}

	return DeployCRDsWithClient(ctx, log, c, cfg)
}

// DeployCRDsWithClient deploys the required CRDs for the DNSManager if/as specified in the given configuration.
func DeployCRDsWithClient(ctx context.Context, log logr.Logger, c client.Client, cfg *config.DNSManagerConfiguration) error {
	var err error

	if !ptr.Deref(cfg.DeployCRDs, false) {
		log.Info("Skipping CRD deployment")
		return nil
	}

	deployedCRDs := set.Set[string]{}
	if ptr.Deref(cfg.ConditionalDeployCRDs, false) {
		deployedCRDs, err = findCRDsDeployedByManagedResources(ctx, c)
		if err != nil {
			return fmt.Errorf("DeployCRDs: could not find already deployed CRDs: %w", err)
		}
	}

	crds, err := requiredCRDs(cfg)
	if err != nil {
		return fmt.Errorf("DeployCRDs: could not get required CRDs: %w", err)
	}
	log.Info("Deploying CRDs")
	for name, desiredCRD := range crds {
		if deployedCRDs.Has(name) {
			log.Info("Skipping deployment of CRD as it is already deployed by ManagedResources", "name", name)
			continue
		}
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: desiredCRD.Name,
			},
		}

		result, err := controllerutils.GetAndCreateOrMergePatch(ctx, c, crd, func() error {
			crd.Labels = desiredCRD.Labels
			crd.Annotations = desiredCRD.Annotations
			if ptr.Deref(cfg.AddShootNoCleanupLabelToCRDs, false) {
				utils.SetLabel(crd, v1beta1constants.ShootNoCleanup, "true")
			}

			crd.Spec = desiredCRD.Spec
			return nil
		})
		if err != nil {
			return fmt.Errorf("DeployCRDs: could not create or update CRD %s: %w", name, err)
		}
		log.Info("Deployed CRD", "name", name, "result", result)
	}
	return nil
}

func findCRDsDeployedByManagedResources(ctx context.Context, c client.Client) (set.Set[string], error) {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "garden"}, ns); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("findCRDsDeployedByManagedResources: could not get namespace: %w", err)
	}

	crd := apiextensionsv1.CustomResourceDefinition{}
	if err := c.Get(ctx, client.ObjectKey{Name: "managedresources.resources.gardener.cloud"}, &crd); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("findCRDsDeployedByManagedResources: could not get ManagedResource CRD: %w", err)
	}
	mrList := &resourcesv1alpha1.ManagedResourceList{}
	if err := c.List(ctx, mrList, client.InNamespace("garden")); err != nil {
		return nil, fmt.Errorf("findCRDsDeployedByManagedResources: could not list managed resources: %w", err)
	}

	crdNames := set.Set[string]{}
	for _, mr := range mrList.Items {
		for _, item := range mr.Status.Resources {
			if item.APIVersion != "apiextensions.k8s.io/v1" {
				continue
			}
			if item.Kind != "CustomResourceDefinition" {
				continue
			}
			crdNames.Insert(item.Name)
		}
	}
	return crdNames, nil
}

func requiredCRDs(cfg *config.DNSManagerConfiguration) (map[string]*apiextensionsv1.CustomResourceDefinition, error) {
	manifests := []string{
		apidns.DNSAnnotationsCRD,
		apidns.DNSEntriesCRD,
	}

	if ptr.Deref(cfg.Controllers.Source.DNSProviderReplication, false) {
		manifests = append(manifests, apidns.DNSProvidersCRD)
	}

	crdNameToCRD := make(map[string]*apiextensionsv1.CustomResourceDefinition, len(manifests))
	for _, manifest := range manifests {
		crdObj, err := kubernetesutils.DecodeCRD(manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CRD: %w", err)
		}
		crdNameToCRD[crdObj.GetName()] = crdObj
	}
	return crdNameToCRD, nil
}
