// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"fmt"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RecorderWithDeduplication is an event recorder with deduplication capabilities.
type RecorderWithDeduplication interface {
	record.EventRecorder
	// DedupEventf records an event only if the same event message has not been recorded
	// for the given object within the deduplication TTL.
	DedupEventf(client client.Object, eventtype, reason, messageFmt string, args ...interface{})
}

type dedupRecorder struct {
	recorder   record.EventRecorder
	eventCache *ttlcache.Cache[client.ObjectKey, string]
}

// NewDedupRecorder creates a new RecorderWithDeduplication with the given TTL for deduplication.
func NewDedupRecorder(recorder record.EventRecorder, ttl time.Duration) RecorderWithDeduplication {
	return &dedupRecorder{
		recorder: recorder,
		eventCache: ttlcache.New[client.ObjectKey, string](
			ttlcache.WithTTL[client.ObjectKey, string](ttl),
			ttlcache.WithDisableTouchOnHit[client.ObjectKey, string]()),
	}
}

func (d *dedupRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	obj, ok := object.(client.Object)
	if ok {
		d.eventCache.Set(client.ObjectKeyFromObject(obj), message, ttlcache.DefaultTTL)
	}
	d.recorder.Event(object, eventtype, reason, message)
}

func (d *dedupRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	obj, ok := object.(client.Object)
	if ok {
		d.eventCache.Set(client.ObjectKeyFromObject(obj), msg, ttlcache.DefaultTTL)
	}
	d.recorder.Event(object, eventtype, reason, msg)
}

func (d *dedupRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	obj, ok := object.(client.Object)
	if ok {
		msg := fmt.Sprintf(messageFmt, args...)
		d.eventCache.Set(client.ObjectKeyFromObject(obj), msg, ttlcache.DefaultTTL)
	}
	d.recorder.AnnotatedEventf(object, annotations, eventtype, reason, messageFmt, args)
}

func (d *dedupRecorder) DedupEventf(object client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	msg := fmt.Sprintf(messageFmt, args...)
	if item := d.eventCache.Get(client.ObjectKeyFromObject(object)); item == nil || item.Value() != msg {
		d.recorder.Event(object, eventtype, reason, msg)
		d.eventCache.Set(client.ObjectKeyFromObject(object), msg, ttlcache.DefaultTTL)
		d.eventCache.DeleteExpired()
	}
}
