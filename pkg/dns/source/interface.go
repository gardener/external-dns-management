// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type DNSInfo struct {
	Names                     dns.DNSNameSet
	TTL                       *int64
	Interval                  *int64
	Targets                   utils.StringSet
	Text                      utils.StringSet
	OrigRef                   *v1alpha1.EntryReference
	TargetRef                 *v1alpha1.EntryReference
	RoutingPolicy             *v1alpha1.RoutingPolicy
	IPStack                   string
	ResolveTargetsToAddresses *bool
	Ignore                    string
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
	Setup() error

	CreateDNSFeedback(obj resources.Object) DNSFeedback
	GetDNSInfo(logger logger.LogContext, obj resources.ObjectData, current *DNSCurrentState) (*DNSInfo, error)

	Delete(logger logger.LogContext, obj resources.Object) reconcile.Status
	Deleted(logger logger.LogContext, key resources.ClusterObjectKey)
}

type DNSSourceType interface {
	Name() string
	GroupKind() schema.GroupKind
	Create(controller.Interface) (DNSSource, error)
}

type TargetExtraction struct {
	Targets                   utils.StringSet
	Texts                     utils.StringSet
	IPStack                   string
	ResolveTargetsToAddresses *bool
	Ignore                    string
}

type (
	DNSTargetExtractor func(logger logger.LogContext, obj resources.ObjectData, names dns.DNSNameSet) (*TargetExtraction, error)
	DNSSourceCreator   func(controller.Interface) (DNSSource, error)
)

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
