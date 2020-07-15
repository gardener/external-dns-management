/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package source

import (
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/annotation/annotations"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

func SourceReconciler(sourceType DNSSourceType, rtype controller.ReconcilerType) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		source, err := sourceType.Create(c)
		if err != nil {
			return nil, err
		}
		opt, err := c.GetStringOption(OPT_TARGET_REALMS)
		if err != nil {
			opt = ""
		}
		realmtype := access.NewRealmType(dns.REALM_ANNOTATION)
		realms := realmtype.NewRealms(opt)
		c.Infof("target realm(s): %v", realms)
		classes := controller.NewClassesByOption(c, OPT_CLASS, dns.CLASS_ANNOTATION, dns.DEFAULT_CLASS)
		c.SetFinalizerHandler(controller.NewFinalizerForClasses(c, c.GetDefinition().FinalizerName(), classes))
		targetclasses := controller.NewTargetClassesByOption(c, OPT_TARGET_CLASS, dns.CLASS_ANNOTATION, classes)
		slaves := reconcilers.NewSlaveAccess(c, sourceType.Name(), SlaveResources, MasterResourcesType(sourceType.GroupKind()))
		reconciler := &sourceReconciler{
			SlaveAccess:   slaves,
			classes:       classes,
			targetclasses: targetclasses,
			targetrealms:  realms,

			state: c.GetOrCreateSharedValue(KEY_STATE,
				func() interface{} {
					return NewState()
				}).(*state),
			annotations: annotations.GetOrCreateWatches(c),
		}

		reconciler.annotations.RegisterHandler(c, c.GetMainWatchResource().ResourceType().GroupKind(), reconciler)
		reconciler.state.source = source
		reconciler.namespace, _ = c.GetStringOption(OPT_NAMESPACE)
		reconciler.nameprefix, _ = c.GetStringOption(OPT_NAMEPREFIX)
		reconciler.creatorLabelName, _ = c.GetStringOption(OPT_TARGET_CREATOR_LABEL_NAME)
		reconciler.creatorLabelValue, _ = c.GetStringOption(OPT_TARGET_CREATOR_LABEL_VALUE)
		reconciler.ownerid, _ = c.GetStringOption(OPT_TARGET_OWNER_ID)
		reconciler.setIgnoreOwners, _ = c.GetBoolOption(OPT_TARGET_SET_IGNORE_OWNERS)

		excluded, _ := c.GetStringArrayOption(OPT_EXCLUDE)
		reconciler.excluded = utils.NewStringSetByArray(excluded)
		reconciler.Infof("found excluded domains: %v", reconciler.excluded)

		if c.GetMainCluster() == c.GetCluster(TARGET_CLUSTER) {
			reconciler.namespace = ""
			reconciler.nameprefix = ""
		}

		nested, err := reconcilers.NewNestedReconciler(rtype, reconciler)
		if err != nil {
			return nil, err
		}
		reconciler.NestedReconciler = nested
		return reconciler, nil
	}
}

type sourceReconciler struct {
	*reconcilers.NestedReconciler
	*reconcilers.SlaveAccess
	excluded          utils.StringSet
	classes           *controller.Classes
	targetclasses     *controller.Classes
	targetrealms      *access.Realms
	namespace         string
	nameprefix        string
	creatorLabelName  string
	creatorLabelValue string
	ownerid           string
	setIgnoreOwners   bool

	state       *state
	annotations *annotations.State
}

func (this *sourceReconciler) ObjectUpdated(key resources.ClusterObjectKey) {
	this.Infof("requeue %s because of change in annotation resource", key)
	this.EnqueueKey(key)
}

func (this *sourceReconciler) Setup() error {
	this.SlaveAccess.Setup()
	this.state.source.Setup()
	return this.NestedReconciler.Setup()
}

func (this *sourceReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	slaves := this.LookupSlaves(obj.ClusterKey())
	names := utils.StringSet{}
	for _, s := range slaves {
		e := dnsutils.DNSEntry(s).DNSEntry()
		names.Add(e.Spec.DNSName)
	}
	found := &DNSCurrentState{Names: map[string]*DNSState{}, Targets: utils.StringSet{}}
	for n := range names {
		s := this.AssertSingleSlave(logger, obj.ClusterKey(), slaves, dns.DNSNameMatcher(n))
		e := dnsutils.DNSEntry(s).DNSEntry()
		found.Names[n] = &DNSState{DNSEntryStatus: e.Status, CreationTimestamp: e.CreationTimestamp}
		found.Targets.AddAll(e.Spec.Targets)
	}

	info, responsible, err := this.getDNSInfo(logger, obj, this.state.source, found)
	if err != nil {
		obj.Event(core.EventTypeWarning, "reconcile", err.Error())
	}

	if !responsible {
		if len(slaves) > 0 {
			logger.Infof("not responsible anymore, but still found slaves (cleanup required): %v", resources.ObjectArrayToString(slaves...))
			info = &DNSInfo{}
		}
	}

	feedback := this.state.GetFeedbackForObject(obj)

	if info == nil {
		if responsible {
			logger.Debugf("no dns info found")
			err2 := this.annotations.SetActive(obj.ClusterKey(), false)
			if err2 != nil {
				err = err2
			}
		}
		if err != nil {
			return reconcile.Failed(logger, err)
		}
		return reconcile.Succeeded(logger).Stop()
	} else {
		// if not responsible now  it was responsible, therefore cleanup the active state
		err = this.annotations.SetActive(obj.ClusterKey(), responsible)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
	}
	missing := utils.StringSet{}
	obsolete := []resources.Object{}
	obsolete_dns := utils.StringSet{}

	current := []resources.Object{}

	if len(info.Names) > 0 && requireFinalizer(obj, this.SlaveResoures()[0].GetCluster()) {
		err := this.SetFinalizer(obj)
		if err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot set finalizer: %s", err))
		}
	} else {
		err := this.RemoveFinalizer(obj)
		if err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot remove finalizer: %s", err))
		}
	}
	logger.Debugf("found names: %s", info.Names)
outer:
	for dnsname := range info.Names {
		for _, s := range slaves {
			found := dnsutils.DNSEntry(s).DNSEntry().Spec.DNSName
			if found == dnsname {
				continue outer
			}
		}
		missing.Add(dnsname)
	}

	for _, s := range slaves {
		dnsname := dnsutils.DNSEntry(s).DNSEntry().Spec.DNSName
		if !info.Names.Contains(dnsname) {
			obsolete = append(obsolete, s)
			obsolete_dns.Add(dnsname)
		} else {
			current = append(current, s)
		}
	}

	var notifiedErrors []string
	modified := map[string]bool{}
	if len(missing) > 0 {
		if len(info.Targets) > 0 || (info.Text != nil && len(info.Text) > 0) {
			logger.Infof("found missing dns entries: %s", missing)
			for dnsname := range missing {
				err := this.createEntryFor(logger, obj, dnsname, info, feedback)
				if err != nil {
					notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot create dns entry object for %s: %s ", dnsname, err))
				}
			}
		} else {
			logger.Infof("no targets found -> omit creation of missing dns entries: %s", missing)
		}
	}
	if len(obsolete_dns) > 0 {
		logger.Infof("found obsolete dns entries: %s", obsolete_dns)
		for _, o := range obsolete {
			dnsname := dnsutils.DNSEntry(o).DNSEntry().Spec.DNSName
			err := this.deleteEntry(logger, obj, o)
			if err != nil {
				notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot remove dns entry object %q(%s): %s", o.ClusterKey(), dnsname, err))
			}
		}

	}
	if len(current) > 0 {
		for _, o := range current {
			dnsname := dnsutils.DNSEntry(o).DNSEntry().Spec.DNSName
			mod, err := this.updateEntry(logger, info, o)
			modified[dnsname] = mod
			if err != nil {
				notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot update dns entry object %q(%s): %s", o.ClusterKey(), dnsname, err))
			}
		}
	}

	if len(notifiedErrors) > 0 {
		msg := strings.Join(notifiedErrors, ", ")
		if feedback != nil {
			feedback.Failed(logger, "", fmt.Errorf("%s", msg), nil)
		}
		return reconcile.Delay(logger, fmt.Errorf("reconcile failed: %s", msg))
	}

	if feedback != nil {
		threshold := time.Now().Add(-2 * time.Minute)
		for n := range info.Names {
			s := found.Names[n]
			if s != nil && !modified[n] {
				switch s.State {
				case api.STATE_ERROR:
				case api.STATE_INVALID:
				case api.STATE_PENDING:
				case api.STATE_READY:
				default:
					if s.CreationTimestamp.Time.Before(threshold) {
						feedback.Pending(logger, n, "no dns controller running?", s)
					}
				}
			}
		}
		feedback.Succeeded(logger)
	}

	status := this.NestedReconciler.Reconcile(logger, obj)
	if status.IsSucceeded() {
		if len(info.Names) == 0 {
			return status.Stop()
		}
	}
	return status
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
			logger.Warnf("cannot delete entry object %s for %s: %s", s.ObjectName(), dnsutils.DNSEntry(s).GetDNSName(), err)
			failed = true
		} else {
			logger.Infof("delete dns entry for vanished %s(%s)", s.ObjectName(), dnsutils.DNSEntry(s).GetDNSName())
		}
	}
	if failed {
		return reconcile.Delay(logger, nil)
	}

	this.state.DeleteFeedback(key)
	this.state.source.Deleted(logger, key)
	return this.NestedReconciler.Deleted(logger, key)
}

func (this *sourceReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	failed := false
	logger.Infof("entry source is deleting -> delete all dns entries")
	for _, s := range this.Slaves().GetByOwner(obj) {
		logger.Infof("delete dns entry %s(%s)", s.ObjectName(), dnsutils.DNSEntry(s).GetDNSName())
		err := s.Delete()
		if err != nil && !errors.IsNotFound(err) {
			logger.Warnf("cannot delete entry object %s for %s: %s", s.ObjectName(), dnsutils.DNSEntry(s).GetDNSName(), err)
			failed = true
		}
	}
	if failed {
		return reconcile.Delay(logger, nil)
	}

	fb := this.state.GetFeedback(obj.ClusterKey())
	if fb != nil {
		fb.Deleted(logger, "", "deleting dns entries", nil)
		this.state.DeleteFeedback(obj.ClusterKey())
	}
	status := this.state.source.Delete(logger, obj)
	if status.IsSucceeded() {
		status = this.NestedReconciler.Delete(logger, obj)
		if status.IsSucceeded() {
			err := this.RemoveFinalizer(obj)
			if err != nil {
				return reconcile.Delay(logger, err)
			}
		}
	}

	return status
}

////////////////////////////////////////////////////////////////////////////////

func (this *sourceReconciler) createEntryFor(logger logger.LogContext, obj resources.Object, dnsname string, info *DNSInfo, feedback DNSFeedback) error {
	entry := &api.DNSEntry{}
	entry.GenerateName = strings.ToLower(this.nameprefix + obj.GetName() + "-" + obj.GroupKind().Kind + "-")
	if !this.targetclasses.IsDefault() {
		resources.SetAnnotation(entry, CLASS_ANNOTATION, this.targetclasses.Main())
	}
	if !this.targetrealms.IsDefault() {
		resources.SetAnnotation(entry, dns.REALM_ANNOTATION, this.targetrealms.AnnotationValue())
	}
	if this.setIgnoreOwners {
		resources.SetAnnotation(entry, access.ANNOTATION_IGNORE_OWNERS, "true")
	}
	if this.creatorLabelName != "" && this.creatorLabelValue != "" {
		resources.SetLabel(entry, this.creatorLabelName, this.creatorLabelValue)
	}
	if this.ownerid != "" {
		entry.Spec.OwnerId = &this.ownerid
	}
	entry.Spec.DNSName = dnsname
	entry.Spec.Targets = info.Targets.AsArray()
	if info.Text != nil {
		entry.Spec.Text = info.Text.AsArray()
	}
	if this.namespace == "" {
		entry.Namespace = obj.GetNamespace()
	} else {
		entry.Namespace = this.namespace
	}
	entry.Spec.TTL = info.TTL

	e, _ := this.SlaveResoures()[0].Wrap(entry)

	err := this.Slaves().CreateSlave(obj, e)
	if err != nil {
		if feedback != nil {
			feedback.Failed(logger, dnsname, err, nil)
		}
		return err
	}
	obj.Eventf(core.EventTypeNormal, "reconcile", "created dns entry object %s", e.ObjectName())
	logger.Infof("created dns entry object %s", e.ObjectName())
	if feedback != nil {
		feedback.Pending(logger, dnsname, "", nil)
	}
	return nil
}

func (this *sourceReconciler) updateEntry(logger logger.LogContext, info *DNSInfo, obj resources.Object) (bool, error) {
	f := func(o resources.ObjectData) (bool, error) {
		spec := &o.(*api.DNSEntry).Spec
		mod := &utils.ModificationState{}
		var changed bool

		if !this.targetclasses.IsDefault() {
			changed = resources.SetAnnotation(o, CLASS_ANNOTATION, this.targetclasses.Main())
		} else {
			changed = resources.RemoveAnnotation(o, CLASS_ANNOTATION)
		}
		mod.Modify(changed)

		if !this.targetrealms.IsDefault() {
			changed = resources.SetAnnotation(o, dns.REALM_ANNOTATION, this.targetrealms.AnnotationValue())
		} else {
			changed = resources.RemoveAnnotation(o, dns.REALM_ANNOTATION)
		}
		mod.Modify(changed)

		if this.setIgnoreOwners {
			changed = resources.SetAnnotation(o, access.ANNOTATION_IGNORE_OWNERS, "true")
		} else {
			changed = resources.RemoveAnnotation(o, access.ANNOTATION_IGNORE_OWNERS)
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
		var p *string
		if this.ownerid != "" {
			p = &this.ownerid
		}
		mod.AssureStringPtrPtr(&spec.OwnerId, p)
		mod.AssureInt64PtrPtr(&spec.TTL, info.TTL)
		mod.AssureInt64PtrPtr(&spec.CNameLookupInterval, info.Interval)
		mod.AssureStringSet(&spec.Targets, info.Targets)
		if info.Text != nil {
			mod.AssureStringSet(&spec.Text, info.Text)
		} else {
			mod.AssureStringSet(&spec.Text, utils.StringSet{})
		}
		if mod.IsModified() {
			logger.Infof("update entry %s", obj.ObjectName())
		}
		return mod.IsModified(), nil
	}
	return obj.Modify(f)
}

func (this *sourceReconciler) deleteEntry(logger logger.LogContext, obj resources.Object, e resources.Object) error {
	err := e.Delete()
	if err == nil {
		obj.Eventf(core.EventTypeNormal, "reconcile", "deleted dns entry object %s", e.ObjectName())
		logger.Infof("deleted dns entry object %s", e.ObjectName())
	} else {
		if !errors.IsNotFound(err) {
			logger.Errorf("cannot delete dns entry object %s: %s", e.ObjectName(), err)
		} else {
			err = nil
		}
	}
	return err
}
