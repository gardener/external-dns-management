// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"fmt"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// FetchSourceFunc defines a function type that fetches a Kubernetes object (the source)
// for a given reconcile.Request. It returns the fetched object and an error if the fetch fails.
type FetchSourceFunc func(ctx context.Context, request reconcile.Request, entry *v1alpha1.DNSEntry) (client.Object, error)

type eventFeedbackWrapper struct {
	recorder        RecorderWithDeduplication
	handler         handler.EventHandler
	mapper          handler.MapFunc
	fetchSourceFunc FetchSourceFunc
}

// NewEventFeedbackWrapper creates a new event feedback wrapper that wraps an existing event handler.
func NewEventFeedbackWrapper(
	recorder RecorderWithDeduplication,
	handler handler.EventHandler,
	mapper handler.MapFunc,
	sourceFetcher FetchSourceFunc,
) handler.EventHandler {
	return &eventFeedbackWrapper{
		recorder:        recorder,
		handler:         handler,
		mapper:          mapper,
		fetchSourceFunc: sourceFetcher,
	}
}

func (e *eventFeedbackWrapper) Create(ctx context.Context, ev event.TypedCreateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	source, entry := e.fetchSourceAndEntry(ctx, ev.Object)
	if source != nil && entry != nil {
		e.recorder.Eventf(source, "Normal", "DNSEntryCreated", "%s: created dns entry object", entry.Spec.DNSName)
	}
	e.handler.Create(ctx, ev, w)
}

func (e *eventFeedbackWrapper) Update(ctx context.Context, ev event.TypedUpdateEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	source, entryOld := e.fetchSourceAndEntry(ctx, ev.ObjectOld)
	entryNew, ok := ev.ObjectNew.(*v1alpha1.DNSEntry)
	if source != nil && entryOld != nil && ok {
		statusOld := entryOld.Status
		statusNew := entryNew.Status
		dnsName := entryNew.Spec.DNSName
		if statusNew.DNSName != nil {
			dnsName = *entryNew.Status.DNSName
		}
		reason := fmt.Sprintf("DNSEntry%s", statusNew.State)
		if statusNew.State == v1alpha1.StateReady {
			if entryNew.DeletionTimestamp == nil &&
				(statusOld.State != v1alpha1.StateReady || entryOld.Status.ObservedGeneration != entryNew.Status.ObservedGeneration) {
				e.recorder.Eventf(source, "Normal", reason, "%s: %s", dnsName, ptr.Deref(statusNew.Message, ""))
			}
		} else if statusNew.State != "" {
			e.recorder.DedupEventf(source, "Normal", reason, "%s: %s", dnsName, ptr.Deref(statusNew.Message, ""))
		}
	}
	e.handler.Update(ctx, ev, w)
}

func (e *eventFeedbackWrapper) Delete(ctx context.Context, ev event.TypedDeleteEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// no need for an additional event
	e.handler.Delete(ctx, ev, w)
}

func (e *eventFeedbackWrapper) Generic(ctx context.Context, ev event.TypedGenericEvent[client.Object], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.handler.Generic(ctx, ev, w)
}

func (e *eventFeedbackWrapper) fetchSourceAndEntry(ctx context.Context, obj client.Object) (client.Object, *v1alpha1.DNSEntry) {
	requests := e.mapper(ctx, obj)
	if len(requests) != 1 {
		return nil, nil
	}
	entry, ok := obj.(*v1alpha1.DNSEntry)
	if !ok {
		return nil, nil
	}
	source, err := e.fetchSourceFunc(ctx, requests[0], entry)
	if err != nil {
		return nil, nil
	}
	return source, entry
}
