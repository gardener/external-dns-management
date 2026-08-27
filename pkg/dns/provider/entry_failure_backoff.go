// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

// entryFailureBackoffConfig configures the exponential backoff applied to
// persistently failing entries.
type entryFailureBackoffConfig struct {
	// base is the delay applied after the first failure.
	base time.Duration
	// factor is the multiplier applied per additional consecutive failure.
	factor float64
	// max is the upper bound for the backoff delay.
	max time.Duration
}

// entryFailureBackoff tracks consecutive provider update failures per entry and
// derives an increasing retry delay. It is used to reduce the reconciliation
// frequency of entries that keep failing (e.g. because of conflicts).
type entryFailureBackoff struct {
	lock     sync.Mutex
	config   entryFailureBackoffConfig
	failures map[resources.ObjectName]*failureInfo
	// now allows overriding the time source in tests.
	now func() time.Time
}

type failureInfo struct {
	count     int
	nextRetry time.Time
	// fingerprint captures the entry's generation and relevant annotations at
	// the time of the failure. If the entry changes, the backoff is dropped so
	// the change is reconciled immediately.
	fingerprint entryFingerprint
}

// entryFingerprint identifies the mutable, user-relevant state of an entry that
// should reset the failure backoff when changed: the spec generation and all
// annotations in the dns.gardener.cloud group.
type entryFingerprint struct {
	generation  int64
	annotations string
}

// computeEntryFingerprint builds a fingerprint from the object's generation and
// its dns.gardener.cloud annotations.
func computeEntryFingerprint(generation int64, annotations map[string]string) entryFingerprint {
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		if strings.HasPrefix(k, dns.ANNOTATION_GROUP+"/") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(annotations[k])
		sb.WriteByte('\n')
	}
	return entryFingerprint{generation: generation, annotations: sb.String()}
}

func newEntryFailureBackoff(config entryFailureBackoffConfig) *entryFailureBackoff {
	return &entryFailureBackoff{
		config:   config,
		failures: map[resources.ObjectName]*failureInfo{},
		now:      time.Now,
	}
}

// recordFailure registers another failure for the given entry and returns the
// time until which the entry should not trigger a hosted zone reconciliation
// again. The entry's generation and dns.gardener.cloud annotations are captured
// so that a later change resets the backoff.
func (b *entryFailureBackoff) recordFailure(object *dnsutils.DNSEntryObject) time.Time {
	return b.recordFailureFor(object.ObjectName(), computeEntryFingerprint(object.GetGeneration(), object.GetAnnotations()))
}

func (b *entryFailureBackoff) recordFailureFor(name resources.ObjectName, fingerprint entryFingerprint) time.Time {
	b.lock.Lock()
	defer b.lock.Unlock()

	info := b.failures[name]
	if info == nil || info.fingerprint != fingerprint {
		// new entry or the entry changed since the last failure -> restart backoff
		info = &failureInfo{}
		b.failures[name] = info
	}
	info.count++
	info.fingerprint = fingerprint
	info.nextRetry = b.now().Add(b.backoffDuration(info.count))
	return info.nextRetry
}

// clear removes any recorded failure for the given entry, e.g. after a
// successful reconciliation or when the entry is deleted.
func (b *entryFailureBackoff) clear(name resources.ObjectName) {
	b.lock.Lock()
	defer b.lock.Unlock()

	delete(b.failures, name)
}

// blockedUntil reports whether the entry is still within its backoff window and,
// if so, the time until which triggering the hosted zone should be suppressed.
// If the entry changed (different generation or dns.gardener.cloud annotations),
// the backoff is dropped and the entry is reported as not blocked.
func (b *entryFailureBackoff) blockedUntil(object *dnsutils.DNSEntryObject) (time.Time, bool) {
	return b.blockedUntilFor(object.ObjectName(), computeEntryFingerprint(object.GetGeneration(), object.GetAnnotations()))
}

func (b *entryFailureBackoff) blockedUntilFor(name resources.ObjectName, fingerprint entryFingerprint) (time.Time, bool) {
	b.lock.Lock()
	defer b.lock.Unlock()

	info := b.failures[name]
	if info == nil {
		return time.Time{}, false
	}
	if info.fingerprint != fingerprint {
		// entry changed since the last failure -> reconcile the change immediately
		delete(b.failures, name)
		return time.Time{}, false
	}
	if !b.now().Before(info.nextRetry) {
		return time.Time{}, false
	}
	return info.nextRetry, true
}

// backoffDuration returns the delay for the given consecutive failure count
// (>= 1), applying exponential growth capped at max with ±10% jitter.
func (b *entryFailureBackoff) backoffDuration(count int) time.Duration {
	d := float64(b.config.base)
	for i := 1; i < count; i++ {
		d *= b.config.factor
		if d >= float64(b.config.max) {
			d = float64(b.config.max)
			break
		}
	}
	if d > float64(b.config.max) {
		d = float64(b.config.max)
	}
	// apply ±10% jitter to avoid synchronized retries of many entries
	jitter := 1 + (rand.Float64()*0.2 - 0.1) // #nosec G404 -- not used for cryptographic purposes
	return time.Duration(d * jitter)
}
