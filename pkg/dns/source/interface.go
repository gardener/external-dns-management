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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type DNSInfo struct {
	Names    utils.StringSet
	TTL      *int64
	Interval *int64
	Targets  utils.StringSet
	Feedback DNSFeedback
}

type DNSFeedback interface {
	Succeeded()
	Pending(dnsname string, msg string)
	Ready(dnsname string, msg string)
	Invalid(dnsname string, err error)
	Failed(dnsname string, err error)
}

type DNSSource interface {
	Start()
	Setup()

	GetDNSInfo(logger logger.LogContext, obj resources.Object, current *DNSCurrentState) (*DNSInfo, error)

	Delete(logger logger.LogContext, obj resources.Object) reconcile.Status
	Deleted(logger logger.LogContext, key resources.ClusterObjectKey)
}

type DNSSourceType interface {
	Name() string
	GroupKind() schema.GroupKind
	Create(controller.Interface) (DNSSource, error)
}

type DNSTargetExtractor func(logger logger.LogContext, obj resources.Object, current *DNSCurrentState) (utils.StringSet, error)
type DNSSourceCreator func(controller.Interface) (DNSSource, error)

type DNSState struct {
	State             string
	Message           *string
	CreationTimestamp metav1.Time
}

type DNSCurrentState struct {
	Names          map[string]*DNSState
	Targets        utils.StringSet
	AnnotatedNames utils.StringSet
}

func NewDNSSouceTypeForExtractor(name string, kind schema.GroupKind, handler DNSTargetExtractor) DNSSourceType {
	return &handlerdnssourcetype{dnssourcetype{name, kind}, NewDefaultDNSSource(handler, kind)}
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
