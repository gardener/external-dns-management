/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package provider

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

type FinalizerHandler interface {
	SetFinalizer(name resources.Object) error
	RemoveFinalizer(name resources.Object) error
}

type StatusUpdate struct {
	*Entry
	logger   logger.LogContext
	delete   bool
	done     bool
	fhandler FinalizerHandler
}

func NewStatusUpdate(logger logger.LogContext, e *Entry, f FinalizerHandler) DoneHandler {
	//logger.Infof("request update for %s (delete=%t)", e.DNSName(), e.IsDeleting())
	return &StatusUpdate{Entry: e, logger: logger, delete: e.IsDeleting(), fhandler: f}
}

func (this *StatusUpdate) SetInvalid(err error) {
	if !this.done {
		this.done = true
		this.modified = false
		this.fhandler.RemoveFinalizer(this.Entry.object)
		_, err := this.UpdateStatus(this.logger, api.STATE_INVALID, err.Error())
		if err != nil {
			this.logger.Errorf("cannot update: %s", err)
		}
	}
}
func (this *StatusUpdate) Failed(err error) {
	if !this.done {
		this.done = true
		this.modified = false
		this.fhandler.RemoveFinalizer(this.Entry.Object())
		_, err := this.UpdateStatus(this.logger, api.STATE_ERROR, err.Error())
		if err != nil {
			this.logger.Errorf("cannot update: %s", err)
		}
	}
}
func (this *StatusUpdate) Succeeded() {
	if !this.done {
		this.done = true
		this.modified = false
		if this.delete {
			this.logger.Infof("removing finalizer for deleted entry %s", this.DNSName())
			this.fhandler.RemoveFinalizer(this.Entry.Object())
		} else {
			this.Entry.activezone = this.ZoneId()
			this.fhandler.SetFinalizer(this.Entry.Object())
			_, err := this.UpdateStatus(this.logger, api.STATE_READY, "dns entry active")
			if err != nil {
				this.logger.Errorf("cannot update: %s", err)
			}
		}
	}
}
