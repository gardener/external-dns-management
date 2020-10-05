/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package certs

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/certmgmt"
)

type Watchable interface {
	RegisterConsumer(h CertificateConsumerUpdater)
}

type WatchableSource struct {
	sync.Mutex
	handlers []CertificateConsumerUpdater
}

func (this *WatchableSource) RegisterConsumer(h CertificateConsumerUpdater) {
	this.Lock()
	defer this.Unlock()
	for _, o := range this.handlers {
		if o == h {
			return
		}
	}
	this.handlers = append(this.handlers, h)
}

func (this *WatchableSource) NotifyUpdate(info certmgmt.CertificateInfo) {
	this.Lock()
	defer this.Unlock()
	for _, h := range this.handlers {
		h.UpdateCertificate(info)
	}
}
