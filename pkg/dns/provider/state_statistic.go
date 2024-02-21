// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
)

////////////////////////////////////////////////////////////////////////////////
// statistic for state
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateStatistic(statistic *statistic.EntryStatistic) {
	list := this.GetStatisticEntries()
	list.UpdateStatistic(statistic)
}

func (this *state) GetStatisticEntries() EntryList {
	this.lock.RLock()
	defer this.lock.RUnlock()
	list := EntryList{}
	this.entries.AddResponsibleTo(&list)
	this.outdated.AddResponsibleTo(&list)
	return list
}
