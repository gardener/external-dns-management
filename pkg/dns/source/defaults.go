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
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/external-dns-management/pkg/dns"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

////////////////////////////////////////////////////////////////////////////////
// Events
////////////////////////////////////////////////////////////////////////////////

// Events stores events per cluster object key.
type Events struct {
	lock   sync.Mutex
	Events map[resources.ClusterObjectKey]map[string]string
}

// NewEvents creates a new Events object.
func NewEvents() *Events {
	return &Events{Events: map[resources.ClusterObjectKey]map[string]string{}}
}

func (this *Events) HasEvents(key resources.ClusterObjectKey) bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.Events[key] != nil
}

func (this *Events) GetEvents(key resources.ClusterObjectKey) map[string]string {
	this.lock.Lock()
	defer this.lock.Unlock()
	events := this.Events[key]
	if events == nil {
		events = map[string]string{}
		this.Events[key] = events
	}
	return events
}

func (this *Events) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.Deleted(logger, obj.ClusterKey())
	return reconcile.Succeeded(logger)
}

func (this *Events) Deleted(_ logger.LogContext, key resources.ClusterObjectKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	delete(this.Events, key)
}

////////////////////////////////////////////////////////////////////////////////
// DNSSource
////////////////////////////////////////////////////////////////////////////////

type DefaultDNSSource struct {
	handler DNSTargetExtractor
	*Events
}

var _ DNSSource = &DefaultDNSSource{}

func NewDefaultDNSSource(handler DNSTargetExtractor) DefaultDNSSource {
	return DefaultDNSSource{handler, NewEvents()}
}

func (this *DefaultDNSSource) Setup() {
}

func (this *DefaultDNSSource) CreateDNSFeedback(obj resources.Object) DNSFeedback {
	return NewEventFeedback(obj, this.GetEvents(obj.ClusterKey()))
}

func (this *DefaultDNSSource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *DNSCurrentState) (*DNSInfo, error) {
	info := &DNSInfo{}
	info.Names = dns.NewDNSNameSetFromStringSet(current.AnnotatedNames, current.GetSetIdentifier())
	tgts, txts, err := this.handler(logger, obj, info.Names)
	info.Targets = tgts
	info.Text = txts
	return info, err
}

///////////////////////////////////////////////////////////////////////////////

func (this *dnssourcetype) Name() string {
	return this.name
}

func (this *dnssourcetype) GroupKind() schema.GroupKind {
	return this.kind
}

func (this *handlerdnssourcetype) Create(_ controller.Interface) (DNSSource, error) {
	return this, nil
}

func (this *creatordnssourcetype) Create(c controller.Interface) (DNSSource, error) {
	return this.handler(c)
}
