// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/external-dns-management/pkg/dns"
)

// feedbackInterval is the interval for reporting feedback events.
const feedbackInterval = 15 * time.Minute

////////////////////////////////////////////////////////////////////////////////
// Events
////////////////////////////////////////////////////////////////////////////////

// Events stores events per cluster object key.
type Events struct {
	lock   sync.Mutex
	Events map[resources.ClusterObjectKey]map[string]TimestampedMessage
}

// TimestampedMessage stores a message with a timestamp.
type TimestampedMessage struct {
	Message   string
	Timestamp time.Time
}

// NewTimestampedMessage creates a new TimestampedMessage with the current time.
func NewTimestampedMessage(msg string) TimestampedMessage {
	return TimestampedMessage{
		Message:   msg,
		Timestamp: time.Now(),
	}
}

// IsOutdated checks if the message is outdated based on the given duration.
func (m TimestampedMessage) IsOutdated(duration time.Duration) bool {
	if m.Message == "" {
		return false
	}
	return m.Timestamp.Add(duration).Before(time.Now())
}

// NewEvents creates a new Events object.
func NewEvents() *Events {
	return &Events{Events: map[resources.ClusterObjectKey]map[string]TimestampedMessage{}}
}

func (this *Events) HasEvents(key resources.ClusterObjectKey) bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.Events[key] != nil
}

func (this *Events) GetEvents(key resources.ClusterObjectKey) map[string]TimestampedMessage {
	this.lock.Lock()
	defer this.lock.Unlock()
	events := this.Events[key]
	if events == nil {
		events = map[string]TimestampedMessage{}
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

func (this *DefaultDNSSource) Setup() error {
	return nil
}

func (this *DefaultDNSSource) CreateDNSFeedback(obj resources.Object) DNSFeedback {
	return NewEventFeedback(obj, this.GetEvents(obj.ClusterKey()))
}

func (this *DefaultDNSSource) GetDNSInfo(logger logger.LogContext, obj resources.ObjectData, current *DNSCurrentState) (*DNSInfo, error) {
	info := &DNSInfo{}
	info.Names = dns.NewDNSNameSetFromStringSet(current.AnnotatedNames, current.GetSetIdentifier())
	extraction, err := this.handler(logger, obj, info.Names)
	if extraction != nil {
		info.Targets = extraction.Targets
		info.Text = extraction.Texts
		info.IPStack = extraction.IPStack
		info.ResolveTargetsToAddresses = extraction.ResolveTargetsToAddresses
		info.Ignore = extraction.Ignore
	}
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
