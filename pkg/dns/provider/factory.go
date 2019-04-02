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

	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type DNSHandlerCreatorFunction func(logger logger.LogContext, config *DNSHandlerConfig, metrics Metrics) (DNSHandler, error)

type Factory struct {
	typecode string
	create   DNSHandlerCreatorFunction
}

var _ DNSHandlerFactory = &Factory{}

func NewDNSHandlerFactory(typecode string, create DNSHandlerCreatorFunction) DNSHandlerFactory {
	return &Factory{typecode, create}
}

func (this *Factory) IsResponsibleFor(object *dnsutils.DNSProviderObject) bool {
	return object.DNSProvider().Spec.Type == this.TypeCode()
}

func (this *Factory) TypeCode() string {
	return this.typecode
}

func (this *Factory) Create(logger logger.LogContext, config *DNSHandlerConfig, metrics Metrics) (DNSHandler, error) {
	return this.create(logger, config, metrics)
}
