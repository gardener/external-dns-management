// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type DNSEntrySource struct {
	source.DefaultDNSSource
	resources resources.Resources
}

type updateOriginalFeedback struct {
	resources  resources.Resources
	isDNSEntry bool
	objectName resources.ObjectName
	chain      source.DNSFeedback
}

func NewDNSEntrySource(c controller.Interface) (source.DNSSource, error) {
	return &DNSEntrySource{DefaultDNSSource: source.NewDefaultDNSSource(nil), resources: c.GetMainCluster().Resources()}, nil
}

func (this *DNSEntrySource) CreateDNSFeedback(obj resources.Object) source.DNSFeedback {
	eventFeedback := source.NewEventFeedback(obj, this.GetEvents(obj.ClusterKey()))
	return &updateOriginalFeedback{
		resources:  this.resources,
		isDNSEntry: obj.GroupKind().Kind == api.DNSEntryKind,
		objectName: obj.ClusterKey().ObjectName(),
		chain:      eventFeedback,
	}
}

func (this *DNSEntrySource) GetDNSInfo(_ logger.LogContext, obj resources.ObjectData, _ *source.DNSCurrentState) (*source.DNSInfo, error) {
	entry := obj.(*api.DNSEntry)
	name := dnsutils.DNSSetName(entry)

	info := &source.DNSInfo{
		Names:                     dns.NewDNSNameSet(name),
		Targets:                   utils.NewStringSetByArray(entry.Spec.Targets),
		Text:                      utils.NewStringSetByArray(entry.Spec.Text),
		OrigRef:                   entry.Spec.Reference,
		TTL:                       entry.Spec.TTL,
		Interval:                  entry.Spec.CNameLookupInterval,
		RoutingPolicy:             entry.Spec.RoutingPolicy,
		IPStack:                   entry.Annotations[dns.AnnotationIPStack],
		ResolveTargetsToAddresses: entry.Spec.ResolveTargetsToAddresses,
	}
	return info, nil
}

func (f *updateOriginalFeedback) Succeeded(logger logger.LogContext) {
	f.chain.Succeeded(logger)
}

func (f *updateOriginalFeedback) Pending(logger logger.LogContext, dnsname string, msg string, state *source.DNSState) {
	f.setStatus(logger, "Pending", msg, state)
	f.chain.Pending(logger, dnsname, msg, state)
}

func (f *updateOriginalFeedback) Ready(logger logger.LogContext, dnsname string, msg string, state *source.DNSState) {
	f.setStatus(logger, "Ready", msg, state)
	f.chain.Ready(logger, dnsname, msg, state)
}

func (f *updateOriginalFeedback) Invalid(logger logger.LogContext, dnsname string, err error, state *source.DNSState) {
	f.setStatus(logger, "Invalid", err.Error(), state)
	f.chain.Invalid(logger, dnsname, err, state)
}

func (f *updateOriginalFeedback) Failed(logger logger.LogContext, dnsname string, err error, state *source.DNSState) {
	f.setStatus(logger, "Error", err.Error(), state)
	f.chain.Failed(logger, dnsname, err, state)
}

func (f *updateOriginalFeedback) Deleted(logger logger.LogContext, dnsname string, msg string) {
	f.chain.Deleted(logger, dnsname, msg)
}

func (f *updateOriginalFeedback) Created(logger logger.LogContext, dnsname string, name resources.ObjectName) {
	f.chain.Created(logger, dnsname, name)
}

func (f *updateOriginalFeedback) setStatus(logger logger.LogContext, _ string, _ string, dnsState *source.DNSState) {
	if dnsState == nil || !f.isDNSEntry {
		return
	}
	obj, err := f.resources.GetObjectInto(f.objectName, &api.DNSEntry{})
	if err != nil {
		logger.Warnf("Cannot get object %s: %s", f.objectName, err)
		return
	}
	data := obj.Data().(*api.DNSEntry)
	data.Status = dnsState.DNSEntryStatus
	data.Status.ObservedGeneration = data.GetGeneration()
	err = obj.UpdateStatus()
	if err != nil {
		logger.Warnf("Cannot update status for object %s: %s", f.objectName, err)
	}
}
