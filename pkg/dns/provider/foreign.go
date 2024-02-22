// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

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
