/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"net"
	"net/http"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/zones"
	"github.com/gophercloud/gophercloud/pagination"
)

// interface between provider and OpenStack DNS API
type designateClientInterface interface {
	// ForEachZone calls handler for each zone managed by the Designate
	ForEachZone(handler func(zone *zones.Zone) error) error

	// ForEachRecordSet calls handler for each recordset in the given DNS zone
	ForEachRecordSet(zoneID string, handler func(recordSet *recordsets.RecordSet) error) error

	// ForEachRecordSet calls handler for each recordset in the given DNS zone restricted to rrtype
	ForEachRecordSetFilterByTypeAndName(zoneID string, rrtype string, name string, handler func(recordSet *recordsets.RecordSet) error) error

	// CreateRecordSet creates recordset in the given DNS zone
	CreateRecordSet(zoneID string, opts recordsets.CreateOpts) (string, error)

	// UpdateRecordSet updates recordset in the given DNS zone
	UpdateRecordSet(zoneID, recordSetID string, opts recordsets.UpdateOpts) error

	// DeleteRecordSet deletes recordset in the given DNS zone
	DeleteRecordSet(zoneID, recordSetID string) error

	// GetRecordSet gets recordset by its ID
	GetRecordSet(zoneID, recordSetID string, handler func(recordSet *recordsets.RecordSet) error) error
}

// implementation of the designateClientInterface
type designateClient struct {
	serviceClient *gophercloud.ServiceClient
	metrics       provider.Metrics
}

var _ designateClientInterface = &designateClient{}

type authConfig struct {
	AuthURL     string
	Username    string
	DomainName  string
	Password    string
	ProjectName string
	// RegionName is optional
	RegionName string
}

// authenticate in OpenStack and obtain Designate service endpoint
func createDesignateServiceClient(logger logger.LogContext, authConfig *authConfig) (*gophercloud.ServiceClient, error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: authConfig.AuthURL,
		Username:         authConfig.Username,
		Password:         authConfig.Password,
		TenantName:       authConfig.ProjectName,
		DomainName:       authConfig.DomainName,
		AllowReauth:      true,
	}

	logger.Infof("Using OpenStack Keystone at %s", opts.IdentityEndpoint)
	authProvider, err := openstack.NewClient(opts.IdentityEndpoint)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	authProvider.HTTPClient.Transport = transport

	if err = openstack.Authenticate(authProvider, opts); err != nil {
		return nil, err
	}

	eo := gophercloud.EndpointOpts{
		Region: authConfig.RegionName,
	}

	client, err := openstack.NewDNSV2(authProvider, eo)
	if err != nil {
		return nil, err
	}
	logger.Infof("Found OpenStack Designate service at %s", client.Endpoint)
	return client, nil
}

// ForEachZone calls handler for each zone managed by the Designate
func (c designateClient) ForEachZone(handler func(zone *zones.Zone) error) error {
	pager := zones.List(c.serviceClient, zones.ListOpts{})
	rt := provider.M_LISTZONES
	return pager.EachPage(
		func(page pagination.Page) (bool, error) {
			c.metrics.AddRequests(rt, 1)
			rt = provider.M_PLISTZONES
			list, err := zones.ExtractZones(page)
			if err != nil {
				return false, err
			}
			for _, zone := range list {
				err := handler(&zone)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)
}

// ForEachRecordSet calls handler for each recordset in the given DNS zone
func (c designateClient) ForEachRecordSet(zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	return c.ForEachRecordSetFilterByTypeAndName(zoneID, "", "", handler)
}

// ForEachRecordSet calls handler for each recordset in the given DNS zone restricted to rrtype
func (c designateClient) ForEachRecordSetFilterByTypeAndName(zoneID string, rrtype string, name string, handler func(recordSet *recordsets.RecordSet) error) error {
	pager := recordsets.ListByZone(c.serviceClient, zoneID, recordsets.ListOpts{Type: rrtype, Name: name})
	rt := provider.M_LISTRECORDS
	return pager.EachPage(
		func(page pagination.Page) (bool, error) {
			c.metrics.AddRequests(rt, 1)
			rt = provider.M_PLISTRECORDS
			list, err := recordsets.ExtractRecordSets(page)
			if err != nil {
				return false, err
			}
			for _, recordSet := range list {
				err := handler(&recordSet)
				if err != nil {
					return false, err
				}
			}
			return true, nil
		},
	)
}

// CreateRecordSet creates recordset in the given DNS zone
func (c designateClient) CreateRecordSet(zoneID string, opts recordsets.CreateOpts) (string, error) {
	r, err := recordsets.Create(c.serviceClient, zoneID, opts).Extract()
	c.metrics.AddRequests("RecordSets_Create", 1)
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

// UpdateRecordSet updates recordset in the given DNS zone
func (c designateClient) UpdateRecordSet(zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	_, err := recordsets.Update(c.serviceClient, zoneID, recordSetID, opts).Extract()
	c.metrics.AddRequests("RecordSets_Update", 1)
	return err
}

// DeleteRecordSet deletes recordset in the given DNS zone
func (c designateClient) DeleteRecordSet(zoneID, recordSetID string) error {
	err := recordsets.Delete(c.serviceClient, zoneID, recordSetID).ExtractErr()
	c.metrics.AddRequests("RecordSets_Delete", 1)
	return err
}

// GetRecordSet gets single recordset by its ID
func (c designateClient) GetRecordSet(zoneID, recordSetID string, handler func(recordSet *recordsets.RecordSet) error) error {
	rs, err := recordsets.Get(c.serviceClient, zoneID, recordSetID).Extract()
	c.metrics.AddRequests("RecordSets_Get", 1)
	if err != nil {
		return err
	}
	return handler(rs)
}
