// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"fmt"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var keyOwnerState = ctxutil.SimpleKey("owner-state")

type ownerState struct {
	lock       sync.Mutex
	resource   resources.Interface
	controller controller.Interface

	ownerObjectName string
	ownerId         string
	active          bool
	initialized     bool
}

func (s *ownerState) GetActive() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.active
}

func (s *ownerState) SetActive(active bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.active = active
}

func (s *ownerState) Setup(logger logger.LogContext) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.initialized {
		return nil
	}
	s.initialized = true

	if s.ownerObjectName != "" {
		var err error
		retries := 5
		for i := 0; i < retries; i++ {
			if i > 0 {
				time.Sleep(5 * time.Second)
			}
			obj := &api.DNSOwner{}
			_, err = s.resource.GetInto(resources.NewObjectName(s.ownerObjectName), obj)
			if err == nil {
				s.setActiveByObj(logger, obj)
				break
			}
		}
		if err != nil {
			return err
		}
	} else {
		s.active = true
	}

	return nil
}

func (s *ownerState) SetActiveByObj(logger logger.LogContext, owner *api.DNSOwner) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.setActiveByObj(logger, owner)
}

func (s *ownerState) setActiveByObj(logger logger.LogContext, owner *api.DNSOwner) {
	oldActive := s.active
	active := (owner.Spec.Active == nil || *owner.Spec.Active) &&
		owner.Status.Active != nil && *owner.Status.Active &&
		s.ownerId == owner.Spec.OwnerId
	if oldActive != active {
		if logger != nil {
			logger.Infof("switching active state of owner %s(id=%s) to %t", s.ownerObjectName, s.ownerId, active)
		}
		s.active = active
	}
}

func getOrCreateSharedOwnerState(c controller.Interface, allocate bool) (*ownerState, error) {
	ownerId, _ := c.GetStringOption(OPT_TARGET_OWNER_ID)
	ownerObject, _ := c.GetStringOption(OPT_TARGET_OWNER_OBJECT)
	if ownerObject != "" && ownerId == "" {
		return nil, fmt.Errorf("%s must be set if %s is provided", OPT_TARGET_OWNER_ID, OPT_TARGET_OWNER_OBJECT)
	}

	var resource resources.Interface
	if ownerObject != "" {
		var err error
		resource, err = c.GetCluster(TARGET_CLUSTER).Resources().Get(ownerGroupKind)
		if err != nil {
			return nil, err
		}
	}
	state := c.GetEnvironment().GetOrCreateSharedValue(keyOwnerState, func() interface{} {
		return &ownerState{ownerObjectName: ownerObject, ownerId: ownerId, resource: resource}
	}).(*ownerState)

	state.lock.Lock()
	defer state.lock.Unlock()
	if state.controller == nil && allocate {
		state.controller = c
	}

	return state, nil
}

func OwnerReconciler(c controller.Interface) (reconcile.Interface, error) {
	state, err := getOrCreateSharedOwnerState(c, true)
	if err != nil {
		return nil, err
	}
	return &ownerReconciler{controller: c, ownerState: state}, nil
}

var _ reconcile.Interface = &ownerReconciler{}

type ownerReconciler struct {
	reconcile.DefaultReconciler

	controller controller.Interface
	ownerState *ownerState
}

func (r *ownerReconciler) RejectResourceReconcilation(_ cluster.Interface, _ schema.GroupKind) bool {
	return r.ownerState.controller != r.controller
}

func (r *ownerReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if obj.ObjectName().Name() != r.ownerState.ownerObjectName {
		return reconcile.Succeeded(logger)
	}

	owner := obj.Data().(*api.DNSOwner)
	r.ownerState.SetActiveByObj(logger, owner)

	return reconcile.Succeeded(logger)
}

func (r *ownerReconciler) Deleted(logger logger.LogContext, obj resources.ClusterObjectKey) reconcile.Status {
	if obj.Name() != r.ownerState.ownerObjectName {
		return reconcile.Succeeded(logger)
	}

	r.ownerState.SetActive(false)

	return reconcile.Succeeded(logger)
}
