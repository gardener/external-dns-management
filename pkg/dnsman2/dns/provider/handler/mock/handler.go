// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// Handler implements the provider.DNSHandler interface for the mock in-memory DNS provider.
type Handler struct {
	provider.DefaultDNSHandler
	config      provider.DNSHandlerConfig
	mock        *InMemory
	mockConfig  MockConfig
	rateLimiter flowcontrol.RateLimiter
}

var _ provider.DNSHandler = &Handler{}

// MockZone represents a mock DNS zone for testing.
type MockZone struct {
	ZoneSuffix string `json:"zoneSuffix"`
	DNSName    string `json:"dnsName"`
}

// ZoneID returns the dns.ZoneID for the mock zone and account.
func (m MockZone) ZoneID(account string) dns.ZoneID {
	return dns.NewZoneID(ProviderType, account+":"+m.ZoneSuffix+m.DNSName)
}

// MockConfig holds configuration for the mock DNS provider.
type MockConfig struct {
	Account              string     `json:"account"`
	Zones                []MockZone `json:"zones"`
	FailGetZones         bool       `json:"failGetZones"`
	FailDeleteEntry      bool       `json:"failDeleteEntry"`
	LatencyMillis        int        `json:"latencyMillis"`
	SupportRoutingPolicy bool       `json:"supportRoutingPolicy"`
}

var _ provider.DNSHandler = &Handler{}

// GetInMemoryMock allows tests to access mocked DNSHosted Zones by account name.
func GetInMemoryMock(account string) *InMemory {
	testInMemoryLock.Lock()
	defer testInMemoryLock.Unlock()
	return testInMemoryAccounts[account]
}

// GetInMemoryMockByZoneID returns the InMemory mock for a given ZoneID.
func GetInMemoryMockByZoneID(id dns.ZoneID) *InMemory {
	testInMemoryLock.Lock()
	defer testInMemoryLock.Unlock()

	account := strings.SplitN(id.ID, ":", 2)[0]
	return testInMemoryAccounts[account]
}

// GetInMemoryMockNames returns the names of all in-memory mocks sorted alphabetically.
func GetInMemoryMockNames() []string {
	testInMemoryLock.Lock()
	defer testInMemoryLock.Unlock()

	names := make([]string, 0, len(testInMemoryAccounts))
	for name := range testInMemoryAccounts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func addInMemoryMock(account string, mock *InMemory) error {
	testInMemoryLock.Lock()
	defer testInMemoryLock.Unlock()
	if _, exists := testInMemoryAccounts[account]; exists {
		return fmt.Errorf("mock for account %s already exists", account)
	}
	testInMemoryAccounts[account] = mock
	return nil
}

func deleteInMemoryMock(account string) {
	testInMemoryLock.Lock()
	defer testInMemoryLock.Unlock()
	delete(testInMemoryAccounts, account)
}

var (
	testInMemoryAccounts = map[string]*InMemory{}
	testInMemoryLock     sync.Mutex
)

// NewHandler creates a new mock DNS handler with the given configuration.
func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var tmp MockConfig
	err := json.Unmarshal(config.Config.Raw, &tmp)
	if err != nil {
		return nil, fmt.Errorf("unmarshal mock providerConfig failed with: %s", err)
	}

	mock := NewInMemory(tmp.SupportRoutingPolicy)

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *config,
		mock:              mock,
		rateLimiter:       config.RateLimiter,
		mockConfig:        tmp,
	}

	if err := addInMemoryMock(h.mockConfig.Account, h.mock); err != nil {
		return nil, fmt.Errorf("failed to add in-memory mock for account %s: %w", h.mockConfig.Account, err)
	}

	for _, mockZone := range h.mockConfig.Zones {
		if mockZone.DNSName != "" {
			zoneID := mockZone.ZoneID(h.mockConfig.Account).ID
			config.Log.Info("Providing mock DNSZone", "zone", mockZone.DNSName, "zoneID", zoneID)
			isPrivate := strings.Contains(mockZone.ZoneSuffix, ":private")
			hostedZone := provider.NewDNSHostedZone(h.ProviderType(), zoneID, mockZone.DNSName, "", isPrivate)
			mock.AddZone(hostedZone)
		}
	}

	return h, nil
}

// Release removes the in-memory mock for the handler's account.
func (h *Handler) Release() {
	deleteInMemoryMock(h.mockConfig.Account)
}

// GetZones returns the hosted zones for the mock provider.
func (h *Handler) GetZones(_ context.Context) ([]provider.DNSHostedZone, error) {
	if h.mockConfig.FailGetZones {
		return nil, fmt.Errorf("forced error by mockConfig.FailGetZones")
	}
	h.config.RateLimiter.Accept()
	zones := h.mock.GetZones()
	return zones, nil
}

// GetCustomQueryDNSFunc returns a custom DNS query function for the mock provider.
func (h *Handler) GetCustomQueryDNSFunc(_ dns.ZoneID, _ utils.QueryDNSFactoryFunc) (provider.CustomQueryDNSFunc, error) {
	return h.queryDNS, nil
}

// QueryDNS queries DNS records in the mock provider.
func (h *Handler) queryDNS(_ context.Context, zoneID dns.ZoneID, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
	result := h.mock.GetRecordset(zoneID, setName.Normalize(), recordType)
	if result == nil {
		return nil, nil
	}
	return result, nil
}

// ExecuteRequests executes DNS change requests in the mock provider.
func (h *Handler) ExecuteRequests(ctx context.Context, zone provider.DNSHostedZone, requests provider.ChangeRequests) error {
	err := h.executeRequests(ctx, zone, requests)
	if h.mockConfig.LatencyMillis > 0 {
		time.Sleep(time.Duration(h.mockConfig.LatencyMillis) * time.Millisecond)
	}
	return err
}

func (h *Handler) executeRequests(ctx context.Context, zone provider.DNSHostedZone, requests provider.ChangeRequests) error {
	log, err := logr.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logger from context: %w", err)
	}
	log.Info("Executing requests", "zoneID", zone.ZoneID(), "updates", len(requests.Updates))

	var succeeded, failed int
	for rtype, update := range requests.Updates {
		h.config.RateLimiter.Accept()
		var err error
		if h.mockConfig.FailDeleteEntry && update.New == nil {
			err = fmt.Errorf("forced error by mockConfig.FailDeleteEntry")
		} else {
			err = h.mock.Apply(zone.ZoneID(), requests.Name, rtype, update, h.config.Metrics)
		}
		if err != nil {
			failed++
			log.Error(err, "Apply failed")
		} else {
			succeeded++
		}
	}
	if succeeded > 0 {
		log.Info("Succeeded updates for records in zone", "zoneID", zone.ZoneID(), "succeeded", succeeded)
	}
	if failed > 0 {
		log.Info("Failed updates for records in zone", "zoneID", zone.ZoneID(), "failed", failed)
		return fmt.Errorf("%d changed failed", failed)
	}

	return nil
}
