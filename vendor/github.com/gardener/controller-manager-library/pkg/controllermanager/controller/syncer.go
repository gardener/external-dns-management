/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package controller

import (
	"fmt"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

// A sync request is used to synchronize the reconcilation of an object with
// the reconcilations of another ressource handled by the same controller.
// It is started for a dedicated object and triggers its reconcilation
// after all instances of a target resource for a target cluster were
// seen by all reconcilers for this resource.
//
// A Syncher describes the formal handling for a dedcated type of synchronization
// given by a cluster and resource.
// It can be configured at a controller definition and refers to the last selected
// cluster, here.

type Syncers map[string]*Syncer

// Syncer is the specification for sync requests. It has a name and can be used
// to initiate sync requests for a dedicated initiating object. It triggers
// reconcilation of the objects of a dedicated resource for a dedicated cluster.
type Syncer struct {
	name     string
	resource ResourceKey
	cluster  resources.Cluster
}

func NewSyncer(name string, resource ResourceKey, cluster resources.Cluster) *Syncer {
	return &Syncer{name, resource, cluster}
}

func (this *Syncer) newSyncRequest(c *controller, initiator resources.Object) *SyncRequest {
	return &SyncRequest{
		Syncer:     this,
		controller: c,
		initiator:  initiator.ClusterKey(),
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// SyncRequests is the structure stored in controller object used to manage
// sync requests
type SyncRequests struct {
	lock       sync.RWMutex
	controller *controller
	syncers    Syncers
	resources  map[ClusterResourceKey]*Syncer

	requests map[string]map[resources.ClusterObjectKey]*SyncRequest
}

func NewSyncRequests(c *controller) *SyncRequests {
	return &SyncRequests{
		controller: c,
		syncers:    Syncers{},
		resources:  map[ClusterResourceKey]*Syncer{},
		requests:   map[string]map[resources.ClusterObjectKey]*SyncRequest{},
	}
}

func (this *SyncRequests) AddSyncer(s *Syncer) error {
	key := NewClusterResourceKey(s.cluster.GetId(), s.resource.GroupKind().Group, s.resource.GroupKind().Kind)

	this.lock.Lock()
	defer this.lock.Unlock()

	if this.resources[key] != nil {
		return fmt.Errorf("syncer for %q already defined", key)
	}
	this.syncers[s.name] = s
	this.resources[key] = s
	return nil
}

func (this *SyncRequests) Get(name string, initiator resources.Object) (*SyncRequest, error) {
	s := this.syncers[name]
	if s == nil {
		return nil, fmt.Errorf("invalid syncer %q", name)
	}
	this.lock.Lock()
	defer this.lock.Unlock()

	requests := this.requests[name]
	if requests == nil {
		requests = map[resources.ClusterObjectKey]*SyncRequest{}
		this.requests[name] = requests
	}

	cur := requests[initiator.ClusterKey()]
	if cur == nil {
		cur = s.newSyncRequest(this.controller, initiator)
		requests[initiator.ClusterKey()] = cur
	}
	return cur, nil
}

func (this *SyncRequests) Remove(r *SyncRequest) {
	this.lock.Lock()
	defer this.lock.Unlock()
	requests := this.requests[r.name]
	if requests != nil {
		old := requests[r.initiator]
		if old != r {
			return
		}
		delete(requests, r.initiator)
		if len(requests) == 0 {
			delete(this.requests, r.name)
		}
	}
}

func (this *SyncRequests) Synchronize(log logger.LogContext, name string, initiator resources.Object) (bool, error) {
	cur, err := this.Get(name, initiator)
	if err != nil {
		return false, err
	}
	done, err := cur.update(log, initiator)
	if err != nil {
		return false, err
	}
	if done {
		this.Remove(cur)
	}
	return done, nil
}

func (this *SyncRequests) requestHandled(log logger.LogContext, reconciler string, key resources.ClusterObjectKey) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	s := this.resources[GetClusterResourceKey(key)]
	if s != nil {
		log.Debugf("found syncer %s for %s", s.name, GetClusterResourceKey(key))
		for _, r := range this.requests[s.name] {
			log.Debugf("   found request for %s", r.initiator)
			r.handledBy(log, key.ObjectName(), reconciler)
		}
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type SyncPoints map[resources.ObjectName]utils.StringSet

// SyncRequest is a dedicted synchronization request.
// It remembers the resource version of the initiator to be updatable
// in case of intermediate new reconcilations of the initiating object
type SyncRequest struct {
	*Syncer
	lock            sync.Mutex
	controller      *controller
	initiator       resources.ClusterObjectKey
	resourceVersion string

	syncPoints SyncPoints
}

// update updates a sync request for a reconcilation of its initiator.
// if the resource version og the initiator hasn't changed, nothing happens.
// Otherwise all objects of the sync resource must be retriggered.
// Therefore all triggered objects are remembered in a sync point map
// holding all unfinished reconcilers for an object. To save memory for
// the simple case of single reconciler, the reconciler set is omitted and
// substituted by a nil entry in this sync point map, if there is only one
// reconciler.
func (this *SyncRequest) update(log logger.LogContext, initiator resources.Object) (bool, error) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.resourceVersion == initiator.GetResourceVersion() {
		if len(this.syncPoints) == 0 {
			log.Infof("synchronization %s(%s) for %s(%s) done", this.name, this.resource, initiator.ClusterKey(), this.resourceVersion)
			return true, nil
		}
		log.Infof("synchronization %s(%s) for %s(%s) still pending", this.name, this.resource, initiator.ClusterKey(), this.resourceVersion)
		return false, nil
	}
	if this.resourceVersion == "" {
		log.Infof("synchronizing %s(%s) for %s(%s)", this.name, this.resource, initiator, initiator.GetResourceVersion())
	} else {
		log.Infof("resynchronizing %s(%s) for %s(%s->%s)", this.name, this.resource, initiator, this.resourceVersion, initiator.GetResourceVersion())
	}
	this.resourceVersion = initiator.GetResourceVersion()
	reconcilers := this.controller.mappings.Get(this.cluster, this.resource.GroupKind())
	if len(reconcilers) == 0 {
		return false, fmt.Errorf("no reconcilers found for resource %s in %s", this.resource, this.cluster)
	}
	list, err := this.controller.ClusterHandler(this.cluster).resources[this.resource].List()
	if err != nil {
		return false, err
	}
	this.syncPoints = SyncPoints{}
	if len(list) == 0 {
		log.Infof("  no %s found for sync -> done", this.resource)
		return true, nil
	}
	if len(reconcilers) == 1 {
		for _, o := range list {
			this.syncPoints[o.ObjectName()] = nil
		}
	} else {
		for _, o := range list {
			this.syncPoints[o.ObjectName()] = reconcilers.Copy()
		}
	}
	return false, this._requestReconcilations(log)
}

// done checks whether all requested reconcilations have been done
func (this *SyncRequest) done() bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return len(this.syncPoints) == 0
}

func (this *SyncRequest) _requestReconcilations(log logger.LogContext) error {
	log.Infof("  syncing %d %s", len(this.syncPoints), this.resource)
	gk := this.resource.GroupKind()

	id := this.cluster.GetId()
	for n := range this.syncPoints {
		this.controller.EnqueueKey(resources.NewClusterKeyForObject(id, n.ForGroupKind(gk)))
	}
	return nil
}

// handledBy is called to notify a reconcilation by a dedicated reconciler.
// A sync resource is removed from the map of pending syncs, if all
// reconcilers have been notified. If all sync points are done, the reconcilation
// of the initiating object is retriggered.
func (this *SyncRequest) handledBy(log logger.LogContext, name resources.ObjectName, reconciler string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	e, ok := this.syncPoints[name]
	if ok {
		if e != nil {
			e.Remove(reconciler)
		}
		if len(e) == 0 {
			delete(this.syncPoints, name)
		}
	}
	if len(this.syncPoints) == 0 {
		log.Infof("sync reconcilations for %s done -> retrigger object", this.initiator)
		this.controller.EnqueueKey(this.initiator)
	} else {
		log.Debugf("still %d pending sync reconcilations for %s", len(this.syncPoints), this.initiator)

	}
}
