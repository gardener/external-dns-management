/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package dnsprovider

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/utils"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const AnnotationSecretResourceVersion = dns.ANNOTATION_GROUP + "/secretResourceVersion"

func NewSlaveAccessSpec(c controller.Interface) reconcilers.SlaveAccessSpec {
	slaveResources := reconcilers.ClusterResources(source.TARGET_CLUSTER, gkDNSProvider)
	spec := reconcilers.NewSlaveAccessSpec(c, c.GetName(), slaveResources, source.MasterResourcesType(gkDNSProvider))
	spec.Namespace, _ = c.GetStringOption(source.OPT_NAMESPACE)
	return spec
}

func DNSProviderReplicationReconciler(c controller.Interface) (reconcile.Interface, error) {
	opt, err := c.GetStringOption(source.OPT_TARGET_REALMS)
	if err != nil {
		opt = ""
	}
	realmtype := access.NewRealmType(dns.REALM_ANNOTATION)
	realms := realmtype.NewRealms(opt)
	c.Infof("target realm(s): %v", realms)
	classes := controller.NewClassesByOption(c, source.OPT_CLASS, dns.CLASS_ANNOTATION, dns.DEFAULT_CLASS)
	c.SetFinalizerHandler(controller.NewFinalizerForClasses(c, c.GetDefinition().FinalizerName(), classes))
	targetclasses := controller.NewTargetClassesByOption(c, source.OPT_TARGET_CLASS, dns.CLASS_ANNOTATION, classes)
	slaves := reconcilers.NewSlaveAccessBySpec(c, NewSlaveAccessSpec(c))
	gkSecret := resources.NewGroupKind("core", "Secret")
	resMainProviders, err := c.GetMainCluster().Resources().GetByGK(gkDNSProvider)
	if err != nil {
		return nil, err
	}
	resMainSecrets, err := c.GetMainCluster().Resources().GetByGK(gkSecret)
	if err != nil {
		return nil, err
	}
	resTargetSecrets, err := c.GetCluster(source.TARGET_CLUSTER).Resources().GetByGK(gkSecret)
	if err != nil {
		return nil, err
	}

	reconciler := &sourceReconciler{
		SlaveAccess:      slaves,
		resMainProviders: resMainProviders,
		resMainSecrets:   resMainSecrets,
		resTargetSecrets: resTargetSecrets,
		classes:          classes,
		targetclasses:    targetclasses,
		targetrealms:     realms,
	}

	//		reconciler.state.source = xsource
	reconciler.namespace, _ = c.GetStringOption(source.OPT_NAMESPACE)
	reconciler.nameprefix, _ = c.GetStringOption(source.OPT_NAMEPREFIX)
	reconciler.creatorLabelName, _ = c.GetStringOption(source.OPT_TARGET_CREATOR_LABEL_NAME)
	reconciler.creatorLabelValue, _ = c.GetStringOption(source.OPT_TARGET_CREATOR_LABEL_VALUE)

	if c.GetMainCluster() == c.GetCluster(source.TARGET_CLUSTER) {
		return nil, fmt.Errorf("not supported if target cluster is same as default cluster")
	}
	return reconciler, nil
}

type sourceReconciler struct {
	*reconcilers.SlaveAccess
	resMainProviders  resources.Interface
	resMainSecrets    resources.Interface
	resTargetSecrets  resources.Interface
	classes           *controller.Classes
	targetclasses     *controller.Classes
	targetrealms      *access.Realms
	namespace         string
	nameprefix        string
	creatorLabelName  string
	creatorLabelValue string
}

func (this *sourceReconciler) ObjectUpdated(key resources.ClusterObjectKey) {
	this.Infof("requeue %s because of change in annotation resource", key)
	this.EnqueueKey(key)
}

func (this *sourceReconciler) Setup() error {
	this.SlaveAccess.Setup()
	return nil
}

func (this *sourceReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	slaves := this.LookupSlaves(obj.ClusterKey())
	s := this.AssertSingleSlave(logger, obj.ClusterKey(), slaves, nil)

	spec := &dnsutils.DNSProvider(obj).DNSProvider().Spec

	responsible := this.classes.IsResponsibleFor(logger, obj)
	if !responsible {
		if s != nil {
			logger.Infof("not responsible anymore, but still found slave (cleanup required): %s", s.ClusterKey())
			err := this.deleteProvider(logger, obj, s)
			if err != nil {
				return reconcile.Failed(logger, err)
			}
			s = nil
			spec = nil
		}
	}

	if spec != nil {
		if err := this.SetFinalizer(obj); err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot set finalizer: %s", err))
		}
	} else {
		if err := this.RemoveFinalizer(obj); err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot remove finalizer: %s", err))
		}
	}

	if spec == nil {
		return reconcile.Succeeded(logger).Stop()
	}

	if s == nil {
		if err := this.createEntryFor(logger, obj, spec); err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot create provider: %s", err))
		}
	} else {
		if _, err := this.updateEntryFor(logger, obj, spec, s); err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot update provider: %s", err))
		}
	}
	return reconcile.Succeeded(logger)
}

// Deleted is used as fallback, if the source object in another cluster is
//  deleted unexpectedly (by removing the finalizer).
//  It checks whether a slave is still available and deletes it.
func (this *sourceReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	logger.Infof("%s finally deleted", key)
	failed := false
	for _, s := range this.Slaves().GetByOwnerKey(key) {
		err := s.Delete()
		if err != nil && !errors.IsNotFound(err) {
			logger.Warnf("cannot delete provider object %s: %s", s.ObjectName(), err)
			failed = true
		} else {
			logger.Infof("delete dns provider for vanished %s", s.ObjectName())
		}
	}
	if failed {
		return reconcile.Delay(logger, nil)
	}

	return reconcile.Succeeded(logger)
}

func (this *sourceReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	failed := false
	logger.Infof("provider source is deleting -> delete all slaves")
	for _, s := range this.Slaves().GetByOwner(obj) {
		err := this.deleteProvider(logger, obj, s)
		if err != nil && !errors.IsNotFound(err) {
			logger.Warnf("cannot delete provider object %s: %s", s.ObjectName(), err)
			failed = true
		}
	}
	if failed {
		return reconcile.Delay(logger, nil)
	}

	err := this.RemoveFinalizer(obj)
	if err != nil {
		return reconcile.Delay(logger, err)
	}
	return reconcile.Succeeded(logger)
}

func (this *sourceReconciler) createEntryFor(logger logger.LogContext, obj resources.Object, spec *api.DNSProviderSpec) error {
	provider := &api.DNSProvider{}
	provider.GenerateName = strings.ToLower(this.nameprefix + obj.GetName() + "-" + obj.GroupKind().Kind + "-")
	if !this.targetclasses.IsDefault() {
		resources.SetAnnotation(provider, source.CLASS_ANNOTATION, this.targetclasses.Main())
	}
	if !this.targetrealms.IsDefault() {
		resources.SetAnnotation(provider, dns.REALM_ANNOTATION, this.targetrealms.AnnotationValue())
	}
	if this.creatorLabelName != "" && this.creatorLabelValue != "" {
		resources.SetLabel(provider, this.creatorLabelName, this.creatorLabelValue)
	}

	provider.Spec = *spec
	provider.Spec.SecretRef = nil

	if this.namespace == "" {
		provider.Namespace = obj.GetNamespace()
	} else {
		provider.Namespace = this.namespace
	}

	p, _ := this.SlaveResoures()[0].Wrap(provider)

	err := this.Slaves().CreateSlave(obj, p)
	if err != nil {
		return err
	}
	obj.Eventf(core.EventTypeNormal, "reconcile", "created dns provider object %s", p.ObjectName())
	logger.Infof("created dns provider object %s", p.ObjectName())
	return nil
}

func (this *sourceReconciler) updateEntryFor(logger logger.LogContext, obj resources.Object, sourceSpec *api.DNSProviderSpec, slave resources.Object) (bool, error) {
	f := func(o resources.ObjectData) (bool, error) {
		target := o.(*api.DNSProvider)
		targetSpec := &target.Spec
		mod := &utils.ModificationState{}
		var changed bool

		if !this.targetclasses.IsDefault() {
			changed = resources.SetAnnotation(o, source.CLASS_ANNOTATION, this.targetclasses.Main())
		} else {
			changed = resources.RemoveAnnotation(o, source.CLASS_ANNOTATION)
		}
		mod.Modify(changed)

		if !this.targetrealms.IsDefault() {
			changed = resources.SetAnnotation(o, dns.REALM_ANNOTATION, this.targetrealms.AnnotationValue())
		} else {
			changed = resources.RemoveAnnotation(o, dns.REALM_ANNOTATION)
		}
		mod.Modify(changed)

		if this.creatorLabelName != "" {
			if this.creatorLabelValue != "" {
				changed = resources.SetLabel(o, this.creatorLabelName, this.creatorLabelValue)
			} else if this.creatorLabelName != "" {
				changed = resources.RemoveLabel(o, this.creatorLabelName)
			}
			mod.Modify(changed)
		}

		mod.AssureStringValue(&targetSpec.Type, sourceSpec.Type)
		mod.AssureInt64PtrPtr(&targetSpec.DefaultTTL, sourceSpec.DefaultTTL)
		assureDNSSelection(mod, &targetSpec.Domains, sourceSpec.Domains)
		assureDNSSelection(mod, &targetSpec.Zones, sourceSpec.Zones)

		changed, err := this.updateSecretIfNeeded(logger, target, slave, obj)
		if err != nil {
			return false, fmt.Errorf("updating secret failed: %w", err)
		}
		mod.Modify(changed)

		if mod.IsModified() {
			logger.Infof("update provider %s", slave.ObjectName())
		}
		return mod.IsModified(), nil
	}
	return slave.Modify(f)
}

func (this *sourceReconciler) deleteProvider(logger logger.LogContext, obj resources.Object, e resources.Object) error {
	err := e.Delete()
	if err == nil {
		obj.Eventf(core.EventTypeNormal, "reconcile", "deleted dns provider object %s", e.ObjectName())
		logger.Infof("deleted dns provider object %s", e.ObjectName())
	} else {
		if !errors.IsNotFound(err) {
			logger.Errorf("cannot delete dns provider object %s: %s", e.ObjectName(), err)
		} else {
			err = nil
		}
	}
	return err
}

func (this *sourceReconciler) updateSecretIfNeeded(logger logger.LogContext, target *api.DNSProvider, slave, obj resources.Object) (bool, error) {
	source := dnsutils.DNSProvider(obj).DNSProvider()
	if source.Spec.SecretRef == nil {
		mod := resources.RemoveAnnotation(target, AnnotationSecretResourceVersion)
		if target.Spec.SecretRef != nil {
			target.Spec.SecretRef = nil
			mod = true
		}
		msg := "no secret specified"
		logger.Infof(msg)
		_, err := this.updateSourceStatus(source, msg)
		if err != nil {
			return false, err
		}
		return mod, nil
	}

	ns := source.Spec.SecretRef.Namespace
	if ns == "" {
		ns = obj.GetNamespace()
	}
	secretName := resources.NewObjectName(ns, source.Spec.SecretRef.Name)
	secret := &core.Secret{}
	_, err := this.resMainSecrets.GetInto(secretName, secret)
	if err != nil {
		msg := fmt.Sprintf("secret %s not found", secret.Name)
		if !errors.IsNotFound(err) {
			msg = fmt.Sprintf("reading secret %s failed: %s", secret.Name, err)
		}
		logger.Infof(msg)
		_, err := this.updateSourceStatus(source, msg)
		if err != nil {
			return false, err
		}
		found := resources.RemoveAnnotation(target, AnnotationSecretResourceVersion)
		if found {
			// delete data of target secret
			secret.Data = nil
			if err := this.writeTargetSecret(logger, target, slave, secret); err != nil {
				return false, err
			}
		}
		return found, nil
	}
	targetSecretResourceVersion, _ := resources.GetAnnotation(target, AnnotationSecretResourceVersion)
	sourceSecretResourceVersion := secret.ResourceVersion
	if targetSecretResourceVersion == sourceSecretResourceVersion {
		return false, nil
	}
	if err := this.writeTargetSecret(logger, target, slave, secret); err != nil {
		return false, err
	}
	resources.SetAnnotation(target, AnnotationSecretResourceVersion, sourceSecretResourceVersion)
	return true, nil
}

func (this *sourceReconciler) writeTargetSecret(logger logger.LogContext, target *api.DNSProvider, slave resources.Object, secret *core.Secret) error {
	secret.Namespace = target.Namespace
	secret.Name = target.Name
	secret.ResourceVersion = ""
	secret.UID = ""
	resources.SetOwnerReference(secret, slave.GetOwnerReference())
	_, err := this.resTargetSecrets.CreateOrUpdate(secret)
	if err != nil {
		return fmt.Errorf("cannot write secret %s for slave provider %s/%s: %w", secret.Name, target.Namespace, target.Name, err)
	}
	target.Spec.SecretRef = &core.SecretReference{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}
	logger.Infof("written secret %s for slave provider %s/%s", secret.Name, target.Namespace, target.Name)
	return nil
}

func (this *sourceReconciler) updateSourceStatus(source *api.DNSProvider, sourceMsg string) (bool, error) {
	_, mod, err := this.resMainProviders.ModifyStatus(source, func(o resources.ObjectData) (bool, error) {
		s := o.(*api.DNSProvider)
		mod := utils.ModificationState{}
		mod.AssureStringPtrPtr(&s.Status.Message, &sourceMsg)
		mod.AssureStringValue(&s.Status.State, api.STATE_ERROR)
		return mod.IsModified(), nil
	})
	if err != nil {
		return false, fmt.Errorf("cannot update source DNS provider: %w", err)
	}
	return mod, nil
}

func assureDNSSelection(mod *utils.ModificationState, t **api.DNSSelection, s *api.DNSSelection) {
	if s == nil {
		if *t != nil {
			*t = nil
			mod.Modify(true)
		}
	} else {
		if *t == nil || !reflect.DeepEqual(*t, s) {
			**t = *s
			mod.Modify(true)
		}
	}
}
