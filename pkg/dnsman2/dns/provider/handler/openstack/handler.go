// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig

	client designateClientInterface
}

var _ provider.DNSHandler = &handler{}

// NewHandler constructs a new DNSHandler object.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	authConfig, err := readAuthConfig(c)
	if err != nil {
		return nil, err
	}

	serviceClient, err := createDesignateServiceClient(context.Background(), c.Log, authConfig)
	if err != nil {
		return nil, err
	}

	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
		client:            designateClient{serviceClient: serviceClient, metrics: c.Metrics},
	}

	return h, nil
}

func readAuthConfig(c *provider.DNSHandlerConfig) (*clientAuthConfig, error) {
	authURL, err := c.GetRequiredProperty("OS_AUTH_URL", "authURL")
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
func (h *handler) Release() {
}

// GetZones returns a list of hosted zones from the cache.
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var hostedZones []provider.DNSHostedZone

	zoneHandler := func(zone *zones.Zone) error {
		if h.isBlockedZone(zone.ID) {
			log.Info("ignoring blocked zone", "zone", zone.ID)
			return nil
		}

		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zone.ID, dns.NormalizeDomainName(zone.Name), "", false)
		hostedZones = append(hostedZones, hostedZone)
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.client.ForEachZone(ctx, zoneHandler); err != nil {
		return nil, fmt.Errorf("listing DNS zones failed. Details: %s", err.Error())
	}

	return hostedZones, nil
}

func (h *handler) getLogFromContext(ctx context.Context) (logr.Logger, error) {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return log, fmt.Errorf("failed to get logger from context: %w", err)
	}
	log = log.WithValues(
		"provider", h.ProviderType(),
	)
	return log, nil
}

func (h *handler) getAdvancedOptions() config.AdvancedOptions {
	return h.config.GlobalConfig.ProviderAdvancedOptions[ProviderType]
}

func (h *handler) isBlockedZone(zoneID string) bool {
	for _, zone := range h.getAdvancedOptions().BlockedZones {
		if zone == zoneID {
			return true
		}
	}
	return false
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the Openstack Designate provider.
func (h *handler) GetCustomQueryDNSFunc(_ dns.ZoneInfo, factory utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, fmt.Errorf("failed to create default query function: %w", err)
	}
	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		queryResult := defaultQueryFunc.Query(ctx, setName, recordType)
		return queryResult.RecordSet, queryResult.Err
	}, nil
}

// ExecuteRequests applies a given change request to a given hosted zone.
func (h *handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, reqs provider.ChangeRequests) error {
	log, err := h.getLogFromContext(ctx)
	if err != nil {
		return err
	}

	exec := newExecution(log, h, zone)

	var succeeded, failed int
	var errs []error
	for _, r := range reqs.Updates {
		if err := exec.apply(ctx, reqs.Name, r); err != nil {
			failed++
			log.Error(err, "apply failed")
			errs = append(errs, err)
		} else {
			succeeded++
		}
	}

	if succeeded > 0 {
		log.Info("Succeeded updates for records", "zone", zone.ZoneID().ID, "count", succeeded)
	}
	if failed > 0 {
		log.Info("Failed updates for records", "zone", zone.ZoneID().ID, "count", failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to execute change requests for zone %s: %w", zone.ZoneID(), errors.Join(errs...))
	}
	return nil
}
