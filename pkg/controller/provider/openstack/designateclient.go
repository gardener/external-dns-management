// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// interface between provider and OpenStack DNS API
type designateClientInterface interface {
	// ForEachZone calls handler for each zone managed by the Designate
	ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error

	// ForEachRecordSet calls handler for each recordset in the given DNS zone
	ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error

	// ForEachRecordSet calls handler for each recordset in the given DNS zone restricted to rrtype
	ForEachRecordSetFilterByTypeAndName(ctx context.Context, zoneID string, rrtype string, name string, handler func(recordSet *recordsets.RecordSet) error) error

	// CreateRecordSet creates recordset in the given DNS zone
	CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error)

	// UpdateRecordSet updates recordset in the given DNS zone
	UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error

	// DeleteRecordSet deletes recordset in the given DNS zone
	DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error
}

// implementation of the designateClientInterface
type designateClient struct {
	serviceClient *gophercloud.ServiceClient
	metrics       provider.Metrics
}

var _ designateClientInterface = &designateClient{}

type clientAuthConfig struct {
	clientconfig.AuthInfo
	RegionName string
	Insecure   bool
	CACert     string
	ClientCert string
	ClientKey  string
}

// authenticate in OpenStack and obtain Designate service endpoint
func createDesignateServiceClient(ctx context.Context, logger logger.LogContext, clientAuthConfig *clientAuthConfig) (*gophercloud.ServiceClient, error) {
	clientOpts := new(clientconfig.ClientOpts)
	clientOpts.AuthInfo = &clientAuthConfig.AuthInfo
	if clientAuthConfig.ApplicationCredentialSecret != "" {
		clientOpts.AuthType = clientconfig.AuthV3ApplicationCredential
	}
	clientOpts.EnvPrefix = "_NEVER_OVERWRITE_FROM_ENV_"

	ao, err := clientconfig.AuthOptions(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create client auth options: %+v", err)
	}
	ao.AllowReauth = true

	logger.Infof("Using OpenStack Keystone at %s", ao.IdentityEndpoint)
	providerClient, err := openstack.NewClient(ao.IdentityEndpoint)
	if err != nil {
		return nil, err
	}

	tlscfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if clientAuthConfig.CACert != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(clientAuthConfig.CACert))
		tlscfg.RootCAs = caCertPool
	}
	if clientAuthConfig.Insecure {
		tlscfg.InsecureSkipVerify = true
	}

	if clientAuthConfig.ClientCert != "" && clientAuthConfig.ClientKey != "" {
		cert, err := tls.X509KeyPair([]byte(clientAuthConfig.ClientCert), []byte(clientAuthConfig.ClientKey))
		if err != nil {
			return nil, err
		}
		tlscfg.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlscfg,
	}
	providerClient.HTTPClient.Transport = transport

	if err = openstack.Authenticate(ctx, providerClient, *ao); err != nil {
		return nil, err
	}

	eo := gophercloud.EndpointOpts{
		Region: clientAuthConfig.RegionName,
	}

	client, err := openstack.NewDNSV2(providerClient, eo)
	if err != nil {
		return nil, err
	}
	logger.Infof("Found OpenStack Designate service at %s", client.Endpoint)
	return client, nil
}

// ForEachZone calls handler for each zone managed by the Designate
func (c designateClient) ForEachZone(ctx context.Context, handler func(zone *zones.Zone) error) error {
	pager := zones.List(c.serviceClient, zones.ListOpts{})
	rt := provider.M_LISTZONES
	return pager.EachPage(ctx,
		func(_ context.Context, page pagination.Page) (bool, error) {
			c.metrics.AddGenericRequests(rt, 1)
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
func (c designateClient) ForEachRecordSet(ctx context.Context, zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	return c.ForEachRecordSetFilterByTypeAndName(ctx, zoneID, "", "", handler)
}

// ForEachRecordSet calls handler for each recordset in the given DNS zone restricted to rrtype
func (c designateClient) ForEachRecordSetFilterByTypeAndName(ctx context.Context, zoneID string, rrtype string, name string, handler func(recordSet *recordsets.RecordSet) error) error {
	pager := recordsets.ListByZone(c.serviceClient, zoneID, recordsets.ListOpts{Type: rrtype, Name: name})
	rt := provider.M_LISTRECORDS
	return pager.EachPage(ctx,
		func(_ context.Context, page pagination.Page) (bool, error) {
			c.metrics.AddZoneRequests(zoneID, rt, 1)
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
func (c designateClient) CreateRecordSet(ctx context.Context, zoneID string, opts recordsets.CreateOpts) (string, error) {
	r, err := recordsets.Create(ctx, c.serviceClient, zoneID, opts).Extract()
	c.metrics.AddZoneRequests(zoneID, provider.M_CREATERECORDS, 1)
	if err != nil {
		return "", err
	}
	return r.ID, nil
}

// UpdateRecordSet updates recordset in the given DNS zone
func (c designateClient) UpdateRecordSet(ctx context.Context, zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	_, err := recordsets.Update(ctx, c.serviceClient, zoneID, recordSetID, opts).Extract()
	c.metrics.AddZoneRequests(zoneID, provider.M_UPDATERECORDS, 1)
	return err
}

// DeleteRecordSet deletes recordset in the given DNS zone
func (c designateClient) DeleteRecordSet(ctx context.Context, zoneID, recordSetID string) error {
	err := recordsets.Delete(ctx, c.serviceClient, zoneID, recordSetID).ExtractErr()
	c.metrics.AddZoneRequests(zoneID, provider.M_DELETERECORDS, 1)
	return err
}
