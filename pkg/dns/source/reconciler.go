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
	"k8s.io/apimachinery/pkg/api/errors"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	core "k8s.io/api/core/v1"
)

func SourceReconciler(sourceType DNSSourceType, rtype controller.ReconcilerType) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		s, err := sourceType.Create(c)
		if err != nil {
			return nil, err
		}
		reconciler := &sourceReconciler{
			SlaveAccess: reconcilers.NewSlaveAccess(c, sourceType.Name(), SlaveResources, MasterResourcesType(sourceType.GroupKind())),
			source:      s,
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
	excluded   utils.StringSet
	source     DNSSource
	key        string
	namespace  string
	nameprefix string
}

func (this *sourceReconciler) Start() {
	this.SlaveAccess.Start()
	this.source.Start()
	this.NestedReconciler.Start()
}

func (this *sourceReconciler) Setup() {
	this.key, _ = this.GetStringOption(OPT_KEY)
	this.namespace, _ = this.GetStringOption(OPT_NAMESPACE)
	this.nameprefix, _ = this.GetStringOption(OPT_NAMEPREFIX)
	excluded, _ := this.GetStringArrayOption(OPT_EXCLUDE)
	this.excluded = utils.NewStringSetByArray(excluded)
	this.Infof("found excluded domains: %v", this.excluded)

	if this.GetMainCluster() == this.GetCluster(TARGET_CLUSTER) {
		this.namespace = ""
		this.nameprefix = ""
	}
	this.SlaveAccess.Setup()
	this.source.Setup()
	this.NestedReconciler.Setup()
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
		found.Names[n] = &DNSState{e.Status.State, e.Status.Message, e.CreationTimestamp}
		found.Targets.AddAll(e.Spec.Targets)
	}

	info, err := this.getDNSInfo(logger, obj, this.source, found)
	if err != nil {
		obj.Event(core.EventTypeWarning, "reconcile", err.Error())
	}

	if info == nil {
		if err != nil {
			return reconcile.Failed(logger, err)
		}
		return reconcile.Succeeded(logger).Stop()
	}
	missing := utils.StringSet{}
	obsolete := []resources.Object{}
	obsolete_dns := utils.StringSet{}

	current := []resources.Object{}

	if len(info.Names) > 0 && requireFinalizer(obj, this.SlaveResoures()[0].GetCluster()) {
		err := this.SetFinalizer(obj)
		if err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot set finalizer: ", err))
		}
	} else {
		err := this.RemoveFinalizer(obj)
		if err != nil {
			return reconcile.Delay(logger, fmt.Errorf("cannot remove finalizer: ", err))
		}
	}
outer:
	for dns := range info.Names {
		for _, s := range slaves {
			found := dnsutils.DNSEntry(s).DNSEntry().Spec.DNSName
			if found == dns {
				continue outer
			}
		}
		missing.Add(dns)
	}

	for _, s := range slaves {
		dns := dnsutils.DNSEntry(s).DNSEntry().Spec.DNSName
		if !info.Names.Contains(dns) {
			obsolete = append(obsolete, s)
			obsolete_dns.Add(dns)
		} else {
			current = append(current, s)
		}
	}

	failed := false
	var notified_errors []error
	modified := map[string]bool{}
	if len(missing) > 0 {
		if len(info.Targets) > 0 {
			logger.Infof("found missing dns entries: %s", missing)
			for dns := range missing {
				err := this.createEntryFor(logger, obj, dns, info)
				if err != nil {
					notified_errors = append(notified_errors, fmt.Errorf("cannot create dns entry object for %s: %s ", dns, err))
					failed = true
				}
			}
		} else {
			logger.Infof("no targets found -> omit creation of missing dns entries: %s", missing)
		}
	}
	if len(obsolete_dns) > 0 {
		logger.Infof("found obsolete dns entries: %s", obsolete_dns)
		for _, o := range obsolete {
			dns := dnsutils.DNSEntry(o).DNSEntry().Spec.DNSName
			err := this.deleteEntry(logger, obj, o)
			if err != nil {
				notified_errors = append(notified_errors, fmt.Errorf("cannot remove dns entry object %q(%s): %s", o.ClusterKey(), dns, err))
				failed = true
			}
		}

	}
	if len(current) > 0 {
		for _, o := range current {
			dns := dnsutils.DNSEntry(o).DNSEntry().Spec.DNSName
			mod, err := this.updateEntry(logger, info, o)
			modified[dns] = mod
			if err != nil {
				notified_errors = append(notified_errors, fmt.Errorf("cannot update dns entry object %q(%s): %s", o.ClusterKey(), dns, err))
				failed = true
			}
		}
	}

	if failed {
		if len(notified_errors) > 0 {
			err := ""
			sep := ""
			for _, e := range notified_errors {
				err = fmt.Sprintf("%s%s%s", err, sep, e)
				sep = ", "
			}
			if info.Feedback != nil {
				info.Feedback.Failed("", fmt.Errorf("%s", err))
			}
			return reconcile.Delay(logger, fmt.Errorf("reconcile failed: %s", err))
		}
		err := fmt.Errorf("reconcile failed")
		if info.Feedback != nil {
			info.Feedback.Failed("", err)
		}
		return reconcile.Delay(logger, err)
	}

	if info.Feedback != nil {
		threshold := time.Now().Add(-2 * time.Minute)
		for n := range info.Names {
			s := found.Names[n]
			if s != nil && !modified[n] {
				switch s.State {
				case api.STATE_ERROR:
					msg := fmt.Errorf("errornous dns entry")
					if s.Message != nil {
						info.Feedback.Failed(n, fmt.Errorf("%s: %s", msg, *s.Message))
					} else {
						info.Feedback.Failed(n, msg)
					}
				case api.STATE_INVALID:
					msg := fmt.Errorf("dns entry invalid")
					if s.Message != nil {
						info.Feedback.Invalid(n, fmt.Errorf("%s: %s", msg, *s.Message))
					} else {
						info.Feedback.Invalid(n, msg)
					}
				case api.STATE_PENDING:
					msg := fmt.Sprintf("dns entry pending")
					if s.Message != nil {
						info.Feedback.Pending(n, fmt.Sprintf("%s: %s", msg, *s.Message))
					} else {
						info.Feedback.Pending(n, msg)
					}
				case api.STATE_READY:
					if s.Message != nil {
						info.Feedback.Ready(n, *s.Message)
					} else {
						info.Feedback.Ready(n, fmt.Sprintf("dns entry ready"))
					}
				default:
					if s.CreationTimestamp.Time.Before(threshold) {
						info.Feedback.Pending(n, "no dns controller running?")
					}
				}
			}
		}
		info.Feedback.Succeeded()
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
	for _, s := range this.Slaves().GetByKey(key) {
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

	this.source.Deleted(logger, key)
	return this.NestedReconciler.Deleted(logger, key)
}

func (this *sourceReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	failed := false
	logger.Infof("entry source is deleting -> delete all dns entries")
	for _, s := range this.Slaves().Get(obj) {
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

	status := this.source.Delete(logger, obj)
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

func (this *sourceReconciler) createEntryFor(logger logger.LogContext, obj resources.Object, dns string, info *DNSInfo) error {
	entry := &api.DNSEntry{}
	entry.GenerateName = strings.ToLower(this.nameprefix + obj.GetName() + "-" + obj.GroupKind().Kind + "-")
	entry.Spec.DNSName = dns
	entry.Spec.Targets = info.Targets.AsArray()
	if this.namespace == "" {
		entry.Namespace = obj.GetNamespace()
	} else {
		entry.Namespace = this.namespace
	}

	e, _ := this.SlaveResoures()[0].Wrap(entry)

	err := this.Slaves().CreateSlave(obj, e)
	if err != nil {
		if info.Feedback != nil {
			info.Feedback.Failed(dns, err)
		}
		return err
	}
	obj.Eventf(core.EventTypeNormal, "reconcile", "created dns entry object %s", e.ObjectName())
	logger.Infof("created dns entry object %s", e.ObjectName())
	if info.Feedback != nil {
		info.Feedback.Pending(dns, "")
	}
	return nil
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

func (this *sourceReconciler) updateEntry(logger logger.LogContext, info *DNSInfo, obj resources.Object) (bool, error) {
	f := func(o resources.ObjectData) (bool, error) {
		spec := &o.(*api.DNSEntry).Spec
		mod := &utils.ModificationState{}
		mod.AssureInt64PtrPtr(&spec.TTL, info.TTL)
		mod.AssureInt64PtrPtr(&spec.CNameLookupInterval, info.Interval)
		mod.AssureStringSet(&spec.Targets, info.Targets)
		if mod.IsModified() {
			logger.Infof("update entry %s", obj.ObjectName())
		}
		return mod.IsModified(), nil
	}
	return obj.Modify(f)
}
