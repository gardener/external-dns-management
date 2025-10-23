// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lookup

import (
	"container/heap"
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LookupProcessor is an interface that defines methods for processing periodic DNS lookups for DNS entry targets.
type LookupProcessor interface {
	// Upsert inserts or updates a lookup job for the given object name.
	Upsert(ctx context.Context, name client.ObjectKey, results LookupAllResults, interval time.Duration)
	// Delete removes the lookup job for the given object name.
	Delete(name client.ObjectKey)

	// Run starts the lookup processor, which periodically checks for jobs to run.
	Run(ctx context.Context)
	// IsRunning returns true if the processor is running.
	IsRunning() bool
}

// EntryTrigger is an interface that allows triggering reconciliation of a DNS entry.
type EntryTrigger interface {
	// TriggerReconciliation triggers the reconciliation of a DNS entry identified by the given key.
	TriggerReconciliation(ctx context.Context, key client.ObjectKey) error
}

// LookupIPFunc defines a function type for looking up IPs for a hostname.
type LookupIPFunc func(string) ([]net.IP, error)

type lookupHostConfig struct {
	lock                       sync.Mutex
	lookupFunc                 LookupIPFunc
	maxConcurrentLookupsPerJob int
	maxLookupRetries           int
	waitLookupRetry            time.Duration
}

func defaultLookupHostConfig() lookupHostConfig {
	return lookupHostConfig{
		lookupFunc:                 net.LookupIP,
		maxConcurrentLookupsPerJob: 4,
		maxLookupRetries:           5,
		waitLookupRetry:            500 * time.Millisecond,
	}
}

// lookupHost allows to override the default lookup function for testing purposes
var lookupHost = defaultLookupHostConfig()

// SetLookupFunc allows to set a custom lookup function for DNS lookups (mainly for testing purposes).
func SetLookupFunc(lookupFunc LookupIPFunc) {
	lookupHost.lock.Lock()
	defer lookupHost.lock.Unlock()
	lookupHost.lookupFunc = lookupFunc
}

type lookupJob struct {
	objectKey client.ObjectKey // Unique identifier for the job, e.g., namespace/name of the DNSEntry

	lock             sync.Mutex
	oldLookupResults LookupAllResults
	scheduledAt      time.Time
	interval         time.Duration

	running atomic.Bool
}

func (j *lookupJob) updateWithLock(newResults LookupAllResults, interval time.Duration) bool {
	j.lock.Lock()
	defer j.lock.Unlock()
	j.interval = interval
	j.scheduledAt = time.Now().Add(interval)
	return j.updateLookupResult(newResults)
}

// update updates lookup results and returns if resolved IP addresses have changed.
func (j *lookupJob) updateLookupResult(newResults LookupAllResults) bool {
	changed := !j.oldLookupResults.AllIPAddrs.Equal(newResults.AllIPAddrs)
	j.oldLookupResults = newResults
	return changed && !newResults.HasTimeoutError()
}

type lookupQueue []*lookupJob

var _ heap.Interface = &lookupQueue{}

func (q lookupQueue) Len() int {
	return len(q)
}

func (q lookupQueue) Less(i, j int) bool {
	return q[i].scheduledAt.Before(q[j].scheduledAt)
}

func (q lookupQueue) Swap(i, j int) {
	tmp := q[i]
	q[i] = q[j]
	q[j] = tmp
}

func (q *lookupQueue) Push(x any) {
	job := x.(*lookupJob)
	*q = append(*q, job)
}

func (q *lookupQueue) Pop() any {
	old := *q
	n := len(old)
	x := old[n-1]
	*q = old[0 : n-1]
	return x
}

type lookupProcessor struct {
	lock           sync.Mutex
	log            logr.Logger
	checkPeriod    time.Duration
	concurrentJobs int
	slots          chan struct{}
	queue          lookupQueue
	entryTrigger   EntryTrigger
	running        atomic.Bool
	skipped        atomic.Int64
	metrics        LookupMetrics
}

// NewLookupProcessor creates a new lookupProcessor.
func NewLookupProcessor(
	log logr.Logger,
	entryTrigger EntryTrigger,
	concurrentJobs int,
	checkPeriod time.Duration,
) *lookupProcessor {
	return &lookupProcessor{
		log:            log,
		checkPeriod:    checkPeriod,
		concurrentJobs: concurrentJobs,
		slots:          make(chan struct{}, concurrentJobs),
		queue:          lookupQueue{},
		entryTrigger:   entryTrigger,
		metrics:        &defaultLookupMetrics{},
	}
}

// Upsert inserts or updates a lookup job for the given object name.
func (p *lookupProcessor) Upsert(ctx context.Context, name client.ObjectKey, results LookupAllResults, interval time.Duration) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.incrHostnameLookups(name, results)

	idx := p.find(name)
	if idx >= 0 {
		changed := p.queue[idx].updateWithLock(results, interval)
		heap.Fix(&p.queue, idx)
		if changed {
			p.triggerReconciliation(ctx, name)
		}
		return
	}

	job := &lookupJob{
		scheduledAt:      time.Now().Add(interval),
		objectKey:        name,
		interval:         interval,
		oldLookupResults: results,
	}
	heap.Push(&p.queue, job)
	p.metrics.ReportCurrentJobCount(len(p.queue))
}

// Delete removes the lookup job for the given object name.
func (p *lookupProcessor) Delete(name client.ObjectKey) {
	p.lock.Lock()
	defer p.lock.Unlock()

	idx := p.find(name)
	if idx == -1 {
		return
	}
	heap.Remove(&p.queue, idx)
	p.metrics.RemoveJob(name)
	p.metrics.ReportCurrentJobCount(len(p.queue))
}

func (p *lookupProcessor) Run(ctx context.Context) {
	p.running.Store(true)
	defer p.running.Store(false)
	p.log.Info("starting lookup processor", "slots", p.concurrentJobs)

	nextCheck := p.checkPeriod
	for {
		if err := sleep(ctx, nextCheck); err != nil {
			p.log.Info("lookup processor stopped", "error", ctx.Err())
			return
		}
		nextCheckTime, skipped := p.runJob(ctx)
		if skipped != nil {
			p.log.Info("skipped entry as lookup not yet finished", "entry", *skipped)
		}
		nextCheck = time.Until(nextCheckTime)
	}
}

func (p *lookupProcessor) IsRunning() bool {
	return p.running.Load()
}

func (p *lookupProcessor) runJob(ctx context.Context) (time.Time, *client.ObjectKey) {
	var (
		nextCheck time.Time
		job       *lookupJob
	)
	p.lock.Lock()
	if len(p.queue) == 0 {
		nextCheck = time.Now().Add(p.checkPeriod)
	} else {
		idle := time.Until(p.queue[0].scheduledAt)
		if idle < 0 {
			job = p.queue[0]
			job.scheduledAt = time.Now().Add(job.interval)
			heap.Fix(&p.queue, 0)
		}
		nextCheck = p.queue[0].scheduledAt
	}
	p.lock.Unlock()

	if job != nil {
		if !job.running.CompareAndSwap(false, true) {
			p.skipped.Inc()
			p.metrics.IncrSkipped()
			return nextCheck, &job.objectKey
		}
		select {
		case <-ctx.Done():
			job.running.Store(false)
			p.log.Info("warn: lookup cancelled", "error", ctx.Err())
			return nextCheck, &job.objectKey
		case p.slots <- struct{}{}: // Acquire semaphore slot
			go func(j *lookupJob) {
				defer func() {
					j.running.Store(false)
					<-p.slots // Release semaphore slot
				}()
				j.lock.Lock()
				defer j.lock.Unlock()
				newLookupResult := LookupAllHostnamesIPs(ctx, j.oldLookupResults.Hostnames...)
				p.incrHostnameLookups(j.objectKey, newLookupResult)
				if j.updateLookupResult(newLookupResult) {
					p.triggerReconciliation(ctx, j.objectKey)
				}
			}(job)
		}
	}
	return nextCheck, nil
}

func (p *lookupProcessor) incrHostnameLookups(name client.ObjectKey, results LookupAllResults) {
	p.metrics.IncrHostnameLookups(name, len(results.Hostnames), len(results.Errs), results.Duration)
}

func (p *lookupProcessor) triggerReconciliation(ctx context.Context, key client.ObjectKey) {
	if err := p.entryTrigger.TriggerReconciliation(ctx, key); err != nil {
		p.log.Info("warn: failed to trigger entry reconciliatio", "entry", key, "error", err)
	}
	p.metrics.IncrLookupChanged(key)
}

func (p *lookupProcessor) find(name client.ObjectKey) int {
	for i, job := range p.queue {
		if job.objectKey == name {
			return i
		}
	}
	return -1
}

// LookupAllResults holds the results of a lookup for multiple hostnames, including errors and durations.
type LookupAllResults struct {
	Hostnames  []string
	IPv4Addrs  []string
	IPv6Addrs  []string
	Errs       []error
	AllIPAddrs sets.Set[string]
	Duration   time.Duration
}

// HasErrors returns true if any of the lookup results has an error.
func (r LookupAllResults) HasErrors() bool {
	return len(r.Errs) > 0
}

// HasTimeoutError returns true if any of the lookup results has a timeout error.
func (r LookupAllResults) HasTimeoutError() bool {
	for _, err := range r.Errs {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}
	}
	return false
}

// HasOnlyNotFoundError returns true if all lookup results are not found errors.
func (r LookupAllResults) HasOnlyNotFoundError() bool {
	if len(r.Errs) == 0 {
		return false
	}
	for _, err := range r.Errs {
		if dnsErr, ok := err.(*net.DNSError); !ok || !dnsErr.IsNotFound {
			return false
		}
	}
	return true
}

// LookupAllHostnamesIPs performs DNS lookups for multiple hostnames concurrently, returning all results.
func LookupAllHostnamesIPs(ctx context.Context, hostnames ...string) LookupAllResults {
	start := time.Now()
	results := make(chan lookupIPsResult, lookupHost.maxConcurrentLookupsPerJob)
	go func() {
		sem := make(chan struct{}, lookupHost.maxConcurrentLookupsPerJob)
		for _, hostname := range hostnames {
			select {
			case <-ctx.Done():
				results <- lookupIPsResult{err: ctx.Err()}
			case sem <- struct{}{}: // Acquire semaphore slot
				go func(h string) {
					defer func() {
						<-sem // Release semaphore slot
					}()
					results <- lookupIPs(h)
				}(hostname)
			}
		}
	}()

	all := LookupAllResults{Hostnames: hostnames, AllIPAddrs: sets.New[string]()}
	for range len(hostnames) {
		result := <-results
		if result.err != nil {
			all.Errs = append(all.Errs, result.err)
			continue
		}

		for _, addr := range result.ipv4Addrs {
			if all.AllIPAddrs.Has(addr) {
				continue
			}
			all.IPv4Addrs = append(all.IPv4Addrs, addr)
			all.AllIPAddrs.Insert(addr)
		}
		for _, addr := range result.ipv6Addrs {
			if all.AllIPAddrs.Has(addr) {
				continue
			}
			all.IPv6Addrs = append(all.IPv6Addrs, addr)
			all.AllIPAddrs.Insert(addr)
		}
	}
	all.Duration = time.Since(start)
	sort.Strings(all.IPv4Addrs)
	sort.Strings(all.IPv6Addrs)
	return all
}

type lookupIPsResult struct {
	ipv4Addrs []string
	ipv6Addrs []string
	err       error
}

func lookupIPs(hostname string) lookupIPsResult {
	var (
		ips []net.IP
		err error
	)
	lookupHost.lock.Lock()
	lookupFunc := lookupHost.lookupFunc
	lookupHost.lock.Unlock()
	for i := 1; i <= lookupHost.maxLookupRetries; i++ {
		ips, err = lookupFunc(hostname)
		if err == nil || i == lookupHost.maxLookupRetries {
			break
		}
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			break
		}
		time.Sleep(lookupHost.waitLookupRetry)
	}
	if err != nil {
		if _, ok := err.(*net.DNSError); ok {
			return lookupIPsResult{err: err}
		}
		return lookupIPsResult{err: &net.DNSError{
			Err:        err.Error(),
			Name:       hostname,
			IsTimeout:  isTimeoutError(err),
			IsNotFound: isNotFoundError(err),
		}}
	}
	ipv4addrs := make([]string, 0, len(ips))
	ipv6addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip.To4() != nil {
			ipv4addrs = append(ipv4addrs, ip.String())
		} else if ip.To16() != nil {
			ipv6addrs = append(ipv6addrs, ip.String())
		}
	}
	if len(ipv4addrs) == 0 && len(ipv6addrs) == 0 {
		return lookupIPsResult{err: fmt.Errorf("%s has no IPv4/IPv6 address (of %d addresses)", hostname, len(ips))}
	}
	return lookupIPsResult{ipv4Addrs: ipv4addrs, ipv6Addrs: ipv6addrs}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d < 1*time.Microsecond {
		return nil
	} else if d > 30*time.Second {
		d = 30 * time.Second
	}
	t := time.NewTimer(d)
	select {
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

func isNotFoundError(err error) bool {
	if dnsErr, ok := err.(*net.DNSError); ok {
		return dnsErr.IsNotFound
	}
	return false
}
