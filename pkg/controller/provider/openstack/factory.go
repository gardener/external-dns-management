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

package openstack

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

const TYPE_OPENSTACK = "OpenStack"

type Factory struct {
}

var _ provider.DNSHandlerFactory = &Factory{}

func (this *Factory) IsResponsibleFor(object *dnsutils.DNSProviderObject) bool {
	return object.DNSProvider().Spec.Type == TYPE_OPENSTACK
}

func (this *Factory) TypeCode() string {
	return TYPE_OPENSTACK
}

func (this *Factory) Create(logger logger.LogContext, config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	return NewHandler(logger, config)
}
