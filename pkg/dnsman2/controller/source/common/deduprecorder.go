// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RecorderWithDeduplication is an event recorder with deduplication capabilities.
type RecorderWithDeduplication interface {
	events.EventRecorder
	// DedupEventf records an event only if the same event message has not been recorded
	// for the given object within the deduplication TTL.
	DedupEventf(client client.Object, eventtype, reason, action, messageFmt string, args ...any)
}

type dedupRecorder struct {
	recorder   events.EventRecorder
	eventCache *ttlcache.Cache[client.ObjectKey, string]
}

// NewDedupRecorder creates a new RecorderWithDeduplication with the given TTL for deduplication.
func NewDedupRecorder(recorder events.EventRecorder, ttl time.Duration) RecorderWithDeduplication {
	return &dedupRecorder{
		recorder: recorder,
		eventCache: ttlcache.New[client.ObjectKey, string](
			ttlcache.WithTTL[client.ObjectKey, string](ttl),
			ttlcache.WithDisableTouchOnHit[client.ObjectKey, string]()),
	}
}

func (d *dedupRecorder) Eventf(regarding runtime.Object, related runtime.Object, eventtype, reason, action, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	obj, ok := regarding.(client.Object)
	if ok {
		d.eventCache.Set(client.ObjectKeyFromObject(obj), msg, ttlcache.DefaultTTL)
	}
	d.recorder.Eventf(regarding, related, eventtype, reason, action, msg)
}

func (d *dedupRecorder) DedupEventf(object client.Object, eventtype, reason, action, messageFmt string, args ...any) {
	msg := fmt.Sprintf(messageFmt, args...)
	if item := d.eventCache.Get(client.ObjectKeyFromObject(object)); item == nil || item.Value() != msg {
		d.recorder.Eventf(object, nil, eventtype, reason, action, msg)
		d.eventCache.Set(client.ObjectKeyFromObject(object), msg, ttlcache.DefaultTTL)
		d.eventCache.DeleteExpired()
	}
}
