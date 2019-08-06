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

package dnsentry

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns/source"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

type DNSEntrySource struct {
	source.DefaultDNSSource
	resources resources.Resources
}

type updateOriginalFeedback struct {
	logger     logger.LogContext
	resources  resources.Resources
	objectName resources.ObjectName
	chain      source.DNSFeedback
}

func NewDNSEntrySource(c controller.Interface) (source.DNSSource, error) {
	return &DNSEntrySource{DefaultDNSSource: source.DefaultDNSSource{Events: map[resources.ClusterObjectKey]map[string]string{}}, resources: c.GetMainCluster().Resources()}, nil
}

func (this *DNSEntrySource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	data := obj.Data().(*api.DNSEntry)

	eventFeedback := source.NewEventFeedback(logger, obj, this.GetEvents(obj.ClusterKey()))
	info := &source.DNSInfo{
		Names:    utils.NewStringSet(data.Spec.DNSName),
		Targets:  utils.NewStringSetByArray(data.Spec.Targets),
		Text:     utils.NewStringSetByArray(data.Spec.Text),
		TTL:      data.Spec.TTL,
		Interval: data.Spec.CNameLookupInterval,
		Feedback: &updateOriginalFeedback{
			logger:     logger,
			resources:  this.resources,
			objectName: obj.ClusterKey().ObjectName(),
			chain:      eventFeedback,
		},
	}
	return info, nil
}

func (f *updateOriginalFeedback) Succeeded() {
	f.chain.Succeeded()
}

func (f *updateOriginalFeedback) Pending(dnsname string, msg string, state *source.DNSState) {
	f.setStatus("Pending", msg, state)
	f.chain.Pending(dnsname, msg, state)
}

func (f *updateOriginalFeedback) Ready(dnsname string, msg string, state *source.DNSState) {
	f.setStatus("Ready", msg, state)
	f.chain.Ready(dnsname, msg, state)
}

func (f *updateOriginalFeedback) Invalid(dnsname string, err error, state *source.DNSState) {
	f.setStatus("Invalid", err.Error(), state)
	f.chain.Invalid(dnsname, err, state)
}

func (f *updateOriginalFeedback) Failed(dnsname string, err error, state *source.DNSState) {
	f.setStatus("Error", err.Error(), state)
	f.chain.Failed(dnsname, err, state)
}

func (f *updateOriginalFeedback) setStatus(state string, msg string, dnsState *source.DNSState) {
	obj, err := f.resources.GetObjectInto(f.objectName, &api.DNSEntry{})
	if err != nil {
		logger.Warn("Cannot get object %s: %s", f.objectName, err)
		return
	}
	data := obj.Data().(*api.DNSEntry)
	if dnsState != nil {
		data.Status = dnsState.DNSEntryStatus
	} else {
		data.Status = api.DNSEntryStatus{State: state}
		if msg != "" {
			data.Status.Message = &msg
		}
	}
	data.Status.ObservedGeneration = data.GetGeneration()
	err = obj.UpdateStatus()
	if err != nil {
		logger.Warn("Cannot update status for object %s: %s", f.objectName, err)
	}
}
