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
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

////////////////////////////////////////////////////////////////////////////////
// EventFeedback
////////////////////////////////////////////////////////////////////////////////

type EventFeedback struct {
	logger logger.LogContext
	source resources.Object
	events map[string]string
}

func NewEventFeedback(logger logger.LogContext, obj resources.Object, events map[string]string) DNSFeedback {
	return &EventFeedback{logger, obj, events}
}

func (this *EventFeedback) Ready(dnsname, msg string) {
	if msg == "" {
		msg = fmt.Sprintf("dns entry is ready")
	}
	this.event(dnsname, msg)
}

func (this *EventFeedback) Pending(dnsname, msg string) {
	if msg == "" {
		msg = fmt.Sprintf("dns entry is pending")
	}
	this.event(dnsname, msg)
}

func (this *EventFeedback) Failed(dnsname string, err error) {
	if err == nil {
		err = fmt.Errorf("dns entry is errornous")
	}
	this.event(dnsname, err.Error())
}

func (this *EventFeedback) Invalid(dnsname string, msg error) {
	if msg == nil {
		msg = fmt.Errorf("dns entry is invalid")
	}
	this.event(dnsname, msg.Error())
}

func (this *EventFeedback) Succeeded() {
}

func (this *EventFeedback) event(dnsname, msg string) {
	if msg != this.events[dnsname] {
		key := this.source.ClusterKey()
		this.events[dnsname] = msg
		if dnsname != "" {
			this.logger.Infof("event for %q(%s): %s", key, dnsname, msg)
			this.source.Event(v1.EventTypeNormal, "dns-annotation",
				fmt.Sprintf("%s: %s", dnsname, msg))
		} else {
			this.logger.Infof("event for %q: %s", key, msg)
			this.source.Event(v1.EventTypeNormal, "dns-annotation", msg)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// DNSSource
////////////////////////////////////////////////////////////////////////////////

type DefaultDNSSource struct {
	lock    sync.Mutex
	handler DNSTargetExtractor
	kind    schema.GroupKind
	Events  map[resources.ClusterObjectKey]map[string]string
}

var _ DNSSource = &DefaultDNSSource{}

func NewDefaultDNSSource(handler DNSTargetExtractor, kind schema.GroupKind) DefaultDNSSource {
	return DefaultDNSSource{sync.Mutex{}, handler, kind, map[resources.ClusterObjectKey]map[string]string{}}
}

func (this *dnssourcetype) Name() string {
	return this.name
}

func (this *dnssourcetype) GroupKind() schema.GroupKind {
	return this.kind
}

func (this *handlerdnssourcetype) Create(c controller.Interface) (DNSSource, error) {
	return this, nil
}

func (this *creatordnssourcetype) Create(c controller.Interface) (DNSSource, error) {
	return this.handler(c)
}

func (this *DefaultDNSSource) Setup() {
}

func (this *DefaultDNSSource) Start() {
}

func (this *DefaultDNSSource) GetEvents(key resources.ClusterObjectKey) map[string]string {
	this.lock.Lock()
	defer this.lock.Unlock()
	events := this.Events[key]
	if events == nil {
		events = map[string]string{}
		this.Events[key] = events
	}
	return events
}

func (this *DefaultDNSSource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *DNSCurrentState) (*DNSInfo, error) {
	events := this.GetEvents(obj.ClusterKey())
	info := &DNSInfo{Feedback: NewEventFeedback(logger, obj, events)}
	info.Names = current.AnnotatedNames
	info.Targets = this.handler(logger, obj, current)
	return info, nil
}

func (this *DefaultDNSSource) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.Deleted(logger, obj.ClusterKey())
	return reconcile.Succeeded(logger)
}

func (this *DefaultDNSSource) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	delete(this.Events, key)
}
