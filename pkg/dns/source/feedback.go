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
 * limitations under the License.
 *
 */

package source

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	corev1 "k8s.io/api/core/v1"
)

////////////////////////////////////////////////////////////////////////////////
// EventFeedback
////////////////////////////////////////////////////////////////////////////////

type EventFeedback struct {
	source resources.Object
	events map[string]string
}

func NewEventFeedback(obj resources.Object, events map[string]string) DNSFeedback {
	return &EventFeedback{obj, events}
}

func (this *EventFeedback) Ready(logger logger.LogContext, dnsname, msg string, _ *DNSState) {
	if msg == "" {
		msg = "dns entry is ready"
	}
	this.event(logger, dnsname, msg)
}

func (this *EventFeedback) Pending(logger logger.LogContext, dnsname, msg string, _ *DNSState) {
	if msg == "" {
		msg = "dns entry is pending"
	}
	this.event(logger, dnsname, msg)
}

func (this *EventFeedback) Failed(logger logger.LogContext, dnsname string, err error, _ *DNSState) {
	if err == nil {
		err = fmt.Errorf("dns entry is errorneous")
	}
	this.event(logger, dnsname, err.Error())
}

func (this *EventFeedback) Invalid(logger logger.LogContext, dnsname string, msg error, _ *DNSState) {
	if msg == nil {
		msg = fmt.Errorf("dns entry is invalid")
	}
	this.event(logger, dnsname, msg.Error())
}

func (this *EventFeedback) Deleted(logger logger.LogContext, dnsname string, msg string) {
	if msg == "" {
		msg = "dns entry deleted"
	}
	this.event(logger, dnsname, msg)
}

func (this *EventFeedback) Succeeded(_ logger.LogContext) {
}

func (this *EventFeedback) event(logger logger.LogContext, dnsname, msg string) {
	if this.events == nil || msg != this.events[dnsname] {
		key := this.source.ClusterKey()
		this.events[dnsname] = msg
		if dnsname != "" {
			logger.Infof("event for %q(%s): %s", key, dnsname, msg)
			this.source.Event(corev1.EventTypeNormal, "dns-annotation",
				fmt.Sprintf("%s: %s", dnsname, msg))
		} else {
			logger.Infof("event for %q: %s", key, msg)
			this.source.Event(corev1.EventTypeNormal, "dns-annotation", msg)
		}
	}
}

func (this *EventFeedback) Created(logger logger.LogContext, dnsname string, name resources.ObjectName) {
	this.event(logger, dnsname, fmt.Sprintf("created dns entry object %s", name))
}
