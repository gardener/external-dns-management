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

package conversion

import (
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
)

func MarshalDNSSets(local dns.DNSSets) common.DNSSets {
	result := common.DNSSets{}
	for name, dnsset := range local {
		result[name] = MarshalDNSSet(dnsset)
	}
	return result
}

func MarshalDNSSet(local *dns.DNSSet) *common.DNSSet {
	remote := &common.DNSSet{
		DnsName:     local.Name,
		UpdateGroup: local.UpdateGroup,
		Records:     map[string]*common.RecordSet{},
	}
	for typ, rs := range local.Sets {
		remote.Records[typ] = MarshalRecordSet(rs)
	}
	return remote
}

func MarshalRecordSet(local *dns.RecordSet) *common.RecordSet {
	remote := &common.RecordSet{
		Type: local.Type,
		Ttl:  int32(local.TTL),
	}
	for _, v := range local.Records {
		remote.Record = append(remote.Record, &common.RecordSet_Record{Value: v.Value})
	}
	return remote
}

func MarshalPartialDNSSet(local *dns.DNSSet, recordType string) *common.PartialDNSSet {
	return &common.PartialDNSSet{
		DnsName:     local.Name,
		UpdateGroup: local.UpdateGroup,
		RecordType:  recordType,
		RecordSet:   MarshalRecordSet(local.Sets[recordType]),
	}
}

func UnmarshalDNSSets(remote common.DNSSets) dns.DNSSets {
	local := dns.DNSSets{}
	for name, set := range remote {
		local[name] = UnmarshalDNSSet(set)
	}
	return local
}

func UnmarshalDNSSet(remote *common.DNSSet) *dns.DNSSet {
	local := dns.NewDNSSet(remote.DnsName)
	local.UpdateGroup = remote.UpdateGroup

	for typ, rs := range remote.Records {
		local.Sets[typ] = UnmarshalRecordSet(rs)
	}
	return local
}

func UnmarshalRecordSet(rs *common.RecordSet) *dns.RecordSet {
	local := dns.NewRecordSet(rs.Type, int64(rs.Ttl), nil)
	for _, v := range rs.Record {
		local.Add(&dns.Record{Value: v.Value})
	}
	return local
}

func UnmarshalPartialDNSSet(remote *common.PartialDNSSet) *dns.DNSSet {
	local := dns.NewDNSSet(remote.DnsName)
	local.UpdateGroup = remote.UpdateGroup

	local.Sets[remote.RecordType] = UnmarshalRecordSet(remote.RecordSet)
	return local
}

func UnmarshalChangeRequest(remote *common.ChangeRequest, done provider.DoneHandler) (*provider.ChangeRequest, error) {
	local := &provider.ChangeRequest{
		Type: remote.Change.RecordType,
		Done: done,
	}
	change := UnmarshalPartialDNSSet(remote.Change)
	switch remote.Action {
	case common.ChangeRequest_CREATE:
		local.Action = provider.R_CREATE
		local.Addition = change
	case common.ChangeRequest_UPDATE:
		local.Action = provider.R_UPDATE
		local.Addition = change
	case common.ChangeRequest_DELETE:
		local.Action = provider.R_DELETE
		local.Deletion = change
	default:
		return nil, fmt.Errorf("invalid action: %d", remote.Action)
	}
	return local, nil
}

func MarshalChangeRequest(local *provider.ChangeRequest) (*common.ChangeRequest, error) {
	remote := &common.ChangeRequest{}
	switch local.Action {
	case provider.R_CREATE:
		remote.Action = common.ChangeRequest_CREATE
		remote.Change = MarshalPartialDNSSet(local.Addition, local.Type)
	case provider.R_UPDATE:
		remote.Action = common.ChangeRequest_UPDATE
		remote.Change = MarshalPartialDNSSet(local.Addition, local.Type)
	case provider.R_DELETE:
		remote.Action = common.ChangeRequest_DELETE
		remote.Change = MarshalPartialDNSSet(local.Deletion, local.Type)
	default:
		return nil, fmt.Errorf("invalid action: %s", local.Action)
	}
	return remote, nil
}
