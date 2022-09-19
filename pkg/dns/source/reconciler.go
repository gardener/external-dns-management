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
	"reflect"
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

func NewSlaveAccessSpec(c controller.Interface, sourceType DNSSourceType) reconcilers.SlaveAccessSpec {
	spec := reconcilers.NewSlaveAccessSpec(c, sourceType.Name(), SlaveResources, MasterResourcesType(sourceType.GroupKind()))
	spec.Namespace, _ = c.GetStringOption(OPT_NAMESPACE)
	return spec
}

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
		slaves := reconcilers.NewSlaveAccessBySpec(c, NewSlaveAccessSpec(c, sourceType))
		ownerState, err := getOrCreateSharedOwnerState(c, false)
		if err != nil {
			return nil, err
		}

		reconciler := &sourceReconciler{
			SlaveAccess:   slaves,
			classes:       classes,
			targetclasses: targetclasses,
			targetrealms:  realms,

			state: c.GetOrCreateSharedValue(KEY_STATE,
				func() interface{} {
					return NewState(ownerState)
				}).(*state),
			annotations: annotations.GetOrCreateWatches(c),
		}

		reconciler.annotations.RegisterHandler(c, sourceType.GroupKind(), reconciler)
		reconciler.state.source = source
		reconciler.namespace, _ = c.GetStringOption(OPT_NAMESPACE)
		reconciler.nameprefix, _ = c.GetStringOption(OPT_NAMEPREFIX)
		reconciler.creatorLabelName, _ = c.GetStringOption(OPT_TARGET_CREATOR_LABEL_NAME)
		reconciler.creatorLabelValue, _ = c.GetStringOption(OPT_TARGET_CREATOR_LABEL_VALUE)
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
	setIgnoreOwners   bool

	state       *state
	annotations *annotations.State
}

func (this *sourceReconciler) ObjectUpdated(key resources.ClusterObjectKey) {
	this.Infof("requeue %s because of change in annotation resource", key)
	this.EnqueueKey(key)
}

func (this *sourceReconciler) Setup() error {
	err := this.state.ownerState.Setup(this)
	if err != nil {
		return err
	}
	this.SlaveAccess.Setup()
	this.state.source.Setup()
	return this.NestedReconciler.Setup()
}

func (this *sourceReconciler) lookupSlavesResponsible(logger logger.LogContext, key resources.ClusterObjectKey) []resources.Object {
	var slavesResponsible []resources.Object
	for _, obj := range this.LookupSlaves(key) {
		if !this.targetclasses.IsResponsibleFor(logger, obj) {
			continue
		}
		slavesResponsible = append(slavesResponsible, obj)
	}
	return slavesResponsible
}

func (this *sourceReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	slaves := this.lookupSlavesResponsible(logger, obj.ClusterKey())
	names := dns.DNSNameSet{}
	for _, s := range slaves {
		names.Add(dnsutils.DNSEntry(s).DNSSetName())
	}
	found := &DNSCurrentState{Names: map[dns.DNSSetName]*DNSState{}, Targets: utils.StringSet{}}
	for n := range names {
		s := this.AssertSingleSlave(logger, obj.ClusterKey(), slaves, dnsutils.DNSSetNameMatcher(n))
		e := dnsutils.DNSEntry(s).DNSEntry()
		found.Names[n] = &DNSState{DNSEntryStatus: e.Status, CreationTimestamp: e.CreationTimestamp}
		found.Targets.AddAll(e.Spec.Targets)
	}

	info, responsible, err := this.getDNSInfo(logger, obj, this.state.source, found)
	if err != nil {
		obj.Event(core.EventTypeWarning, "reconcile", err.Error())
	}

	this.state.SetDep(obj.ClusterKey(), this.usedRef(obj, info))
	if !responsible {
		if len(slaves) > 0 {
			logger.Infof("not responsible anymore, but still found slaves (cleanup required): %v", resources.ObjectArrayToString(slaves...))
			info = &DNSInfo{}
		}
	}

	feedback := this.state.CreateFeedbackForObject(obj)

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
	missing := dns.DNSNameSet{}
	obsolete := []resources.Object{}
	obsolete_dns := dns.DNSNameSet{}

	current := []resources.Object{}

	if len(info.Names) > 0 && RequireFinalizer(obj, this.SlaveResoures()[0].GetCluster()) {
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
	for name := range info.Names {
		for _, s := range slaves {
			slaveName := dnsutils.DNSEntry(s).DNSSetName()
			if slaveName == name {
				continue outer
			}
		}
		missing.Add(name)
	}

	for _, s := range slaves {
		slaveName := dnsutils.DNSEntry(s).DNSSetName()
		if !info.Names.Contains(slaveName) {
			obsolete = append(obsolete, s)
			obsolete_dns.Add(slaveName)
		} else {
			current = append(current, s)
		}
	}

	var notifiedErrors []string
	modified := map[dns.DNSSetName]bool{}
	if len(missing) > 0 {
		if len(info.Targets) > 0 || len(info.Text) > 0 || info.OrigRef != nil {
			logger.Infof("found missing dns entries: %s", missing)
			for name := range missing {
				err := this.createEntryFor(logger, obj, name, info, feedback)
				if err != nil {
					notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot create dns entry object for %s: %s ", name, err))
				}
			}
		} else {
			logger.Infof("no targets found -> omit creation of missing dns entries: %s", missing)
		}
	}
	if len(obsolete_dns) > 0 {
		logger.Infof("found obsolete dns entries: %s", obsolete_dns)
		for _, o := range obsolete {
			name := dnsutils.DNSEntry(o).DNSSetName()
			err := this.deleteEntry(logger, o, name, feedback)
			if err != nil {
				notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot remove dns entry object %q(%s): %s", o.ClusterKey(), name, err))
			}
		}

	}
	if len(current) > 0 {
		for _, o := range current {
			name := dnsutils.DNSEntry(o).DNSSetName()
			mod, err := this.updateEntryFor(logger, obj, info, o)
			modified[name] = mod
			if err != nil {
				notifiedErrors = append(notifiedErrors, fmt.Sprintf("cannot update dns entry object %q(%s): %s", o.ClusterKey(), name, err))
			}
		}
	}

	for key := range this.state.GetUsed(obj.ClusterKey()) {
		this.EnqueueKey(key)
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
						feedback.Pending(logger, n.String(), "no dns controller running?", s)
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
//
//	deleted unexpectedly (by removing the finalizer).
//	It checks whether a slave is still available and deletes it.
func (this *sourceReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	// For unclear reasons, k8s client lister spuriously can "forget" an object for some seconds (seen with K8S 1.17.7).
	// As a mitigation, the cache and the kube-apiserver are checked again here.
	if _, err := this.GetCachedObject(key); err == nil {
		logger.Infof("deleted call for source %s delayed, as still in cache", key)
		return reconcile.Recheck(logger, err, 60*time.Second)
	}
	if _, err := this.GetObject(key); err == nil {
		logger.Infof("deleted call for source %s delayed, as still active", key)
		return reconcile.Recheck(logger, err, 60*time.Second)
	}

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
	if !this.classes.IsResponsibleFor(logger, obj) {
		return reconcile.Succeeded(logger)
	}

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
		fb.Deleted(logger, "", "deleting dns entries")
		this.state.DeleteFeedback(obj.ClusterKey())
	}
	status := this.state.source.Delete(logger, obj)
	if status.IsSucceeded() {
		this.state.SetDep(obj.ClusterKey(), nil)
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

func ref(r *api.EntryReference) string {
	return fmt.Sprintf("%s/%s", r.Namespace, r.Name)
}

func (this *sourceReconciler) usedRef(obj resources.Object, info *DNSInfo) *resources.ClusterObjectKey {
	if info != nil && info.OrigRef != nil && info.TargetRef == nil {
		namespace := info.OrigRef.Namespace
		if this.namespace == "" {
			namespace = obj.GetNamespace()
		}
		ref := resources.NewClusterKey(obj.GetCluster().GetId(), entryGroupKind, namespace, info.OrigRef.Name)
		return &ref
	}
	return nil
}

func (this *sourceReconciler) mapRef(obj resources.Object, info *DNSInfo) {
	if info.OrigRef != nil && info.TargetRef == nil {
		key := resources.NewClusterKey(obj.GetCluster().GetId(), entryGroupKind, info.OrigRef.Namespace, info.OrigRef.Name)
		slaves := this.LookupSlaves(key)
		info.TargetRef = &api.EntryReference{}
		if len(slaves) == 1 {
			// DNSEntry always has exactly one slave
			info.TargetRef.Name = slaves[0].GetName()
			info.TargetRef.Namespace = slaves[0].GetNamespace()
			return
		}
		info.TargetRef.Name = info.OrigRef.Name + "-not-found"
		if this.namespace == "" {
			info.TargetRef.Namespace = obj.GetNamespace()
		} else {
			info.TargetRef.Namespace = this.namespace
		}
	}
}

func (this *sourceReconciler) createEntryFor(logger logger.LogContext, obj resources.Object, name dns.DNSSetName, info *DNSInfo, feedback DNSFeedback) error {
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
	if this.state.ownerState.ownerId != "" {
		entry.Spec.OwnerId = &this.state.ownerState.ownerId
	}
	entry.Spec.DNSName = name.DNSName
	this.mapRef(obj, info)
	if info.TargetRef != nil {
		if info.OrigRef != nil {
			logger.Infof("mapping entry reference %s to %s", ref(info.OrigRef), ref(info.TargetRef))
		} else {
			logger.Infof("using target reference %s", ref(info.TargetRef))
		}
		entry.Spec.Reference = info.TargetRef
	} else {
		entry.Spec.Targets = info.Targets.AsArray()
		if info.Text != nil {
			entry.Spec.Text = info.Text.AsArray()
		}
	}

	if this.namespace == "" {
		entry.Namespace = obj.GetNamespace()
	} else {
		entry.Namespace = this.namespace
	}
	entry.Spec.TTL = info.TTL
	entry.Spec.RoutingPolicy = info.RoutingPolicy

	e, _ := this.SlaveResoures()[0].Wrap(entry)

	err := this.Slaves().CreateSlave(obj, e)
	if err != nil {
		if feedback != nil {
			feedback.Failed(logger, name.String(), err, nil)
		}
		return err
	}
	if feedback != nil {
		feedback.Created(logger, name.String(), e.ObjectName())
	} else {
		logger.Infof("created dns entry object %s", e.ObjectName())
	}
	if feedback != nil {
		feedback.Pending(logger, name.String(), "", nil)
	}
	return nil
}

func (this *sourceReconciler) updateEntryFor(logger logger.LogContext, obj resources.Object, info *DNSInfo, slave resources.Object) (bool, error) {
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
		if this.state.ownerState.ownerId != "" {
			p = &this.state.ownerState.ownerId
		}
		mod.AssureStringPtrPtr(&spec.OwnerId, p)
		mod.AssureInt64PtrPtr(&spec.TTL, info.TTL)
		if !reflect.DeepEqual(spec.RoutingPolicy, info.RoutingPolicy) {
			spec.RoutingPolicy = info.RoutingPolicy
			mod.Modify(true)
		}
		mod.AssureInt64PtrPtr(&spec.CNameLookupInterval, info.Interval)
		targets := info.Targets
		text := info.Text

		this.mapRef(obj, info)
		if info.TargetRef != nil {
			if spec.Reference == nil ||
				spec.Reference.Name != info.TargetRef.Name || spec.Reference.Namespace != info.TargetRef.Namespace {
				spec.Reference = info.TargetRef
				targets = nil
				text = nil
				mod.Modify(true)
			}
		} else {
			if spec.Reference != nil {
				spec.Reference = nil
				mod.Modify(true)
			}
		}
		mod.AssureStringSet(&spec.Targets, targets)
		mod.AssureStringSet(&spec.Text, text)
		if mod.IsModified() {
			logger.Infof("update entry %s", slave.ObjectName())
		}
		return mod.IsModified(), nil
	}
	return slave.Modify(f)
}

func (this *sourceReconciler) deleteEntry(logger logger.LogContext, e resources.Object, name dns.DNSSetName, feedback DNSFeedback) error {
	err := e.Delete()
	if err == nil {
		msg := fmt.Sprintf("deleted dns entry object %s", e.ObjectName())
		if feedback != nil {
			feedback.Deleted(logger, name.String(), msg)
		} else {
			logger.Info(msg)
		}
	} else {
		if !errors.IsNotFound(err) {
			logger.Errorf("cannot delete dns entry object %s: %s", e.ObjectName(), err)
		} else {
			err = nil
		}
	}
	return err
}
