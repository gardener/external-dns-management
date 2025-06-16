// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lookup

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MockLookupHostResult is a structure that holds the result of a mock DNS lookup.
type MockLookupHostResult struct {
	IPs []net.IP
	Err error
}

// MockLookupHost is a mock implementation of net.LookupHost for testing purposes.
type MockLookupHost struct {
	delay            time.Duration
	lookupMap        map[string]MockLookupHostResult
	lock             sync.Mutex
	lookupCount      map[string]int
	stopped          atomic.Bool
	retryMap         map[string]int
	serverFailureMap map[string]int
}

// NewMockLookupHost creates a new instance of MockLookupHost with the provided lookup map.
func NewMockLookupHost(lookupMap map[string]MockLookupHostResult) *MockLookupHost {
	return &MockLookupHost{
		lookupMap:   lookupMap,
		lookupCount: map[string]int{},
		stopped:     atomic.Bool{},
	}
}

func (lh *MockLookupHost) LookupHost(hostname string) ([]net.IP, error) {
	time.Sleep(lh.delay)
	lh.lock.Lock()
	if !lh.stopped.Load() {
		lh.lookupCount[hostname] += 1
	}
	if lh.serverFailureMap != nil && lh.serverFailureMap[hostname] > 0 {
		lh.serverFailureMap[hostname]--
		lh.lock.Unlock()
		return nil, &net.DNSError{
			Err:       "server failure",
			IsTimeout: true,
			Server:    "mock",
		}
	}
	if lh.retryMap != nil && lh.retryMap[hostname] > 0 {
		lh.retryMap[hostname]--
		lh.lock.Unlock()
		time.Sleep(lh.delay)
		return nil, &net.DNSError{
			Err:       "i/o timeout",
			IsTimeout: true,
			Server:    "mock",
		}
	}
	result, ok := lh.lookupMap[hostname]
	lh.lock.Unlock()
	if !ok {
		return nil, &net.DNSError{
			Err:        "no such host",
			IsNotFound: true,
			Server:     "mock",
		}
	}
	return result.IPs, result.Err
}

func (lh *MockLookupHost) Stop() {
	lh.stopped.Store(true)
}

type nullTrigger struct{}

// NewNullTrigger creates a new nullTrigger instance that does nothing on reconciliation.
func NewNullTrigger() EntryTrigger {
	return &nullTrigger{}
}

func (t *nullTrigger) TriggerReconciliation(_ context.Context, _ client.ObjectKey) error {
	return nil
}
