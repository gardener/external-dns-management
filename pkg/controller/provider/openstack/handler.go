// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// Handler is the main DNSHandler struct.
type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	ctx    context.Context

	client designateClientInterface
}

var _ provider.DNSHandler = &Handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	authConfig, err := readAuthConfig(config)
	if err != nil {
		return nil, err
	}

	serviceClient, err := createDesignateServiceClient(config.Context, config.Logger, authConfig)
	if err != nil {
		return nil, err
	}

	h := Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
		ctx:               config.Context,
		client:            designateClient{serviceClient: serviceClient, metrics: config.Metrics},
	}

	h.cache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return &h, nil
}

func readAuthConfig(c *provider.DNSHandlerConfig) (*clientAuthConfig, error) {
	authURL, err := c.GetRequiredProperty("OS_AUTH_URL")
	if err != nil {
		return nil, err
	}

	applicationCredentialID := c.GetProperty("OS_APPLICATION_CREDENTIAL_ID", "applicationCredentialID")
	applicationCredentialName := c.GetProperty("OS_APPLICATION_CREDENTIAL_NAME", "applicationCredentialName")
	applicationCredentialSecret := c.GetProperty("OS_APPLICATION_CREDENTIAL_SECRET", "applicationCredentialSecret")
	username := c.GetProperty("OS_USERNAME", "username")
	password := c.GetProperty("OS_PASSWORD", "password")
	if applicationCredentialID != "" || applicationCredentialName != "" {
		if applicationCredentialSecret == "" {
			return nil, fmt.Errorf("'OS_APPLICATION_CREDENTIAL_SECRET' (or 'applicationCredentialSecret') is required if 'OS_APPLICATION_CREDENTIAL_ID' or 'OS_APPLICATION_CREDENTIAL_NAME' is given")
		}
		if applicationCredentialID == "" && applicationCredentialName != "" {
			if username == "" {
				return nil, fmt.Errorf("OS_USERNAME' (or 'username') is required if 'OS_APPLICATION_CREDENTIAL_NAME' is given")
			}
		}
		if password != "" {
			return nil, fmt.Errorf("'OS_PASSWORD' (or 'password)' is not allowed if application credentials are used")
		}
	} else {
		if username == "" {
			return nil, fmt.Errorf("'OS_USERNAME' (or 'username') is required if application credentials are not used")
		}
		if password == "" {
			return nil, fmt.Errorf("'OS_PASSWORD' (or 'password') is required if application credentials are not used")
		}
	}

	domainName := c.GetProperty("OS_DOMAIN_NAME", "domainName")
	domainID := c.GetProperty("OS_DOMAIN_ID", "domainID")
	projectName := c.GetProperty("OS_PROJECT_NAME", "tenantName")
	projectID := c.GetProperty("OS_PROJECT_ID", "tenantID")
	userDomainName := c.GetProperty("OS_USER_DOMAIN_NAME", "userDomainName")
	userDomainID := c.GetProperty("OS_USER_DOMAIN_ID", "userDomainID")
	// optional restriction to region
	regionName := c.GetProperty("OS_REGION_NAME")
	// optional CA Certificate for keystone
	caCert := c.GetProperty("CACERT", "caCert")
	// optional Client Certificate
	clientCert := c.GetProperty("CLIENTCERT", "clientCert")
	clientKey := c.GetProperty("CLIENTKEY", "clientKey")
	insecure := strings.ToLower(c.GetProperty("INSECURE", "insecure"))

	authConfig := clientAuthConfig{
		AuthInfo: clientconfig.AuthInfo{
			AuthURL:                     authURL,
			ApplicationCredentialID:     applicationCredentialID,
			ApplicationCredentialName:   applicationCredentialName,
			ApplicationCredentialSecret: applicationCredentialSecret,
			Username:                    username,
			Password:                    password,
			DomainName:                  domainName,
			DomainID:                    domainID,
			ProjectName:                 projectName,
			ProjectID:                   projectID,
			UserDomainID:                userDomainID,
			UserDomainName:              userDomainName,
		},
		RegionName: regionName,
		CACert:     caCert,
		ClientCert: clientCert,
		ClientKey:  clientKey,
		Insecure:   insecure == "true" || insecure == "yes",
	}

	return &authConfig, nil
}

// Release releases the zone cache.
func (h *Handler) Release() {
	h.cache.Release()
}

// GetZones returns a list of hosted zones from the cache.
func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	blockedZones := h.config.Options.GetBlockedZones()
	hostedZones := provider.DNSHostedZones{}

	zoneHandler := func(zone *zones.Zone) error {
		if blockedZones.Contains(zone.ID) {
			h.config.Logger.Infof("ignoring blocked zone id: %s", zone.ID)
			return nil
		}

		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zone.ID, dns.NormalizeHostname(zone.Name), "", false)
		hostedZones = append(hostedZones, hostedZone)
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.client.ForEachZone(h.ctx, zoneHandler); err != nil {
		return nil, fmt.Errorf("listing DNS zones failed. Details: %s", err.Error())
	}

	return hostedZones, nil
}

// GetZoneState returns the state for a given zone.
func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		switch recordSet.Type {
		case dns.RS_A, dns.RS_AAAA, dns.RS_CNAME, dns.RS_TXT:
			rs := dns.NewRecordSet(recordSet.Type, int64(recordSet.TTL), nil)
			for _, record := range recordSet.Records {
				value := record
				if recordSet.Type == dns.RS_CNAME {
					value = dns.NormalizeHostname(value)
				}
				rs.Add(&dns.Record{Value: value})
			}
			dnssets.AddRecordSetFromProvider(recordSet.Name, rs)
		}
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.client.ForEachRecordSet(h.ctx, zone.Id().ID, recordSetHandler); err != nil {
		return nil, fmt.Errorf("listing DNS zones failed for %s. Details: %s", zone.Id(), err.Error())
	}

	return provider.NewDNSZoneState(dnssets), nil
}

// ExecuteRequests applies a given change request to a given hosted zone.
func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	var succeeded, failed int
	for _, r := range reqs {
		status, rset := exec.buildRecordSet(r)
		if status == bsEmpty || status == bsDryRun {
			continue
		}
		if status == bsInvalidRoutingPolicy {
			err := fmt.Errorf("routing policies unsupported for " + TYPE_CODE)
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		}

		err := exec.apply(h.ctx, r.Action, rset)
		if err != nil {
			failed++
			logger.Infof("Apply failed with %s", err.Error())
			if r.Done != nil {
				r.Done.Failed(err)
			}
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for OpenStack")
		return nil
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zone.Domain(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zone.Domain(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}
