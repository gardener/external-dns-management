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

package dns

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type foreignProvider struct {
	name     resources.ObjectName
	included utils.StringSet
	excluded utils.StringSet
}

func newForeignProvider(name resources.ObjectName) *foreignProvider {
	return &foreignProvider{name: name, included: utils.StringSet{}, excluded: utils.StringSet{}}
}

func (this *foreignProvider) Match(dns string) int {
	ilen := dnsutils.MatchSet(dns, this.included)
	elen := dnsutils.MatchSet(dns, this.excluded)
	return ilen - elen
}

func (this *foreignProvider) Update(logger logger.LogContext, provider *dnsutils.DNSProviderObject) reconcile.Status {
	var included utils.StringSet
	var excluded utils.StringSet

	status := provider.DNSProvider().Status
	if status.Domains.Included != nil {
		included = utils.NewStringSet(status.Domains.Included...)
	}
	if status.Domains.Excluded != nil {
		excluded = utils.NewStringSet(status.Domains.Excluded...)
	}

	if this.included.Equals(included) {
		logger.Infof("included domain changed for foreign provider %q: %s", provider.ObjectName(), included)
		this.included = included
	}

	if this.excluded.Equals(excluded) {
		logger.Infof("excluded domain changed for foreign provider %q: %s", provider.ObjectName(), excluded)
		this.excluded = excluded
	}
	return reconcile.Succeeded(logger)
}
