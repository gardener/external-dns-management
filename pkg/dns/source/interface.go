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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

type DNSInfo struct {
	Names         dns.DNSNameSet
	TTL           *int64
	Interval      *int64
	Targets       utils.StringSet
	Text          utils.StringSet
	OrigRef       *v1alpha1.EntryReference
	TargetRef     *v1alpha1.EntryReference
	RoutingPolicy *v1alpha1.RoutingPolicy
}

type DNSFeedback interface {
	Succeeded(logger logger.LogContext)
	Pending(logger logger.LogContext, dnsname string, msg string, dnsState *DNSState)
	Ready(logger logger.LogContext, dnsname string, msg string, dnsState *DNSState)
	Invalid(logger logger.LogContext, dnsname string, err error, dnsState *DNSState)
	Failed(logger logger.LogContext, dnsname string, err error, dnsState *DNSState)
	Deleted(logger logger.LogContext, dnsname string, msg string)
	Created(logger logger.LogContext, dnsname string, name resources.ObjectName)
}

type DNSSource interface {
	Setup()

	CreateDNSFeedback(obj resources.Object) DNSFeedback
	GetDNSInfo(logger logger.LogContext, obj resources.Object, current *DNSCurrentState) (*DNSInfo, error)

	Delete(logger logger.LogContext, obj resources.Object) reconcile.Status
	Deleted(logger logger.LogContext, key resources.ClusterObjectKey)
}

type DNSSourceType interface {
	Name() string
	GroupKind() schema.GroupKind
	Create(controller.Interface) (DNSSource, error)
}

type DNSTargetExtractor func(logger logger.LogContext, obj resources.Object, names dns.DNSNameSet) (targets utils.StringSet, texts utils.StringSet, err error)
type DNSSourceCreator func(controller.Interface) (DNSSource, error)

type DNSState struct {
	v1alpha1.DNSEntryStatus
	CreationTimestamp metav1.Time
}

type DNSCurrentState struct {
	Names                  map[dns.DNSSetName]*DNSState
	Targets                utils.StringSet
	AnnotatedNames         utils.StringSet
	AnnotatedRoutingPolicy *v1alpha1.RoutingPolicy
}

func (s *DNSCurrentState) GetSetIdentifier() string {
	if s.AnnotatedRoutingPolicy == nil {
		return ""
	}
	return s.AnnotatedRoutingPolicy.SetIdentifier
}

func NewDNSSouceTypeForExtractor(name string, kind schema.GroupKind, handler DNSTargetExtractor) DNSSourceType {
	return &handlerdnssourcetype{dnssourcetype{name, kind}, NewDefaultDNSSource(handler)}
}

func NewDNSSouceTypeForCreator(name string, kind schema.GroupKind, handler DNSSourceCreator) DNSSourceType {
	return &creatordnssourcetype{dnssourcetype{name, kind}, handler}
}

type dnssourcetype struct {
	name string
	kind schema.GroupKind
}

type handlerdnssourcetype struct {
	dnssourcetype
	DefaultDNSSource
}

type creatordnssourcetype struct {
	dnssourcetype
	handler DNSSourceCreator
}
