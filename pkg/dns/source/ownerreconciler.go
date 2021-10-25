/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package source

import (
	"fmt"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var keyOwnerState = ctxutil.SimpleKey("owner-state")

type ownerState struct {
	lock     sync.Mutex
	resource resources.Interface

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
			obj := &api.DNSOwner{}
			_, err = s.resource.GetInto(resources.NewObjectName(s.ownerObjectName), obj)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}
			s.SetActiveByObj(logger, obj)
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
	oldActive := s.GetActive()
	active := (owner.Spec.Active == nil || *owner.Spec.Active) &&
		owner.Status.Active != nil && *owner.Status.Active &&
		s.ownerId == owner.Spec.OwnerId
	if oldActive != active {
		if logger != nil {
			logger.Infof("switching active state of owner %s(id=%s) to %t", s.ownerObjectName, s.ownerId, active)
		}
		s.SetActive(active)
	}
}

func getOrCreateSharedOwnerState(c controller.Interface) (*ownerState, error) {
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
	state := c.GetOrCreateSharedValue(keyOwnerState, func() interface{} {
		return &ownerState{ownerObjectName: ownerObject, ownerId: ownerId, resource: resource}
	}).(*ownerState)
	return state, nil
}

func OwnerReconciler(c controller.Interface) (reconcile.Interface, error) {
	state, err := getOrCreateSharedOwnerState(c)
	if err != nil {
		return nil, err
	}
	return &ownerReconciler{ownerState: state}, nil
}

var _ reconcile.Interface = &ownerReconciler{}

type ownerReconciler struct {
	reconcile.DefaultReconciler

	ownerState *ownerState
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
