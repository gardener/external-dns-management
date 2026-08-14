// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/coredns/corefile-migration/migration/corefile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	globalnetworkingv1 "github.com/googlecloudplatform/google-distributed-cloud-apis/pkg/apis/public/global/networking/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gdcclient "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	dnsconst "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/dns"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Handler struct {
	provider.DefaultDNSHandler
	config  provider.DNSHandlerConfig
	cache   provider.ZoneCache
	project string
	client  client.WithWatch
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	var err error

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *config,
	}

	gdchConfigStr := h.config.Properties[constants.GDCHConfigJSONField]
	if gdchConfigStr == "" {
		return nil, fmt.Errorf("%s required in secret", constants.GDCHConfigJSONField)
	}

	serviceAccountStr := h.config.Properties[constants.ServiceAccountJSONField]
	if serviceAccountStr == "" {
		return nil, fmt.Errorf("%s required in secret", constants.ServiceAccountJSONField)
	}

	gdcConfig := &gdcclient.OrgClusterConfig{}
	if err := json.Unmarshal([]byte(gdchConfigStr), gdcConfig); err != nil {
		return nil, fmt.Errorf("failed to parse GDC Config %w", err)
	}

	serviceAccount := &auth.ServiceAccount{}
	if err := json.Unmarshal([]byte(serviceAccountStr), serviceAccount); err != nil {
		return nil, fmt.Errorf("failed to parse service account %w", err)
	}

	scheme := runtime.NewScheme()
	if err := globalnetworkingv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add global networking types to kubernetes client: %w", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add core types to kubernetes client: %w", err)
	}

	h.client, err = gdcclient.Get(gdcConfig, serviceAccount, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to create Global kubeClient %w", err)
	}

	h.project = serviceAccount.Project
	h.cache, err = config.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

// GetZones gets DNSHostedZones from cache.
func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(_ provider.ZoneCache) (provider.DNSHostedZones, error) {
	// Update metrics of list zone call.
	rt := provider.M_LISTZONES
	h.config.Metrics.AddGenericRequests(rt, 1)
	h.config.RateLimiter.Accept()

	return h.getManagedDNSZones()
}

func (h *Handler) getManagedDNSZones() (provider.DNSHostedZones, error) {
	zones := provider.DNSHostedZones{}

	managedZoneList := &globalnetworkingv1.ManagedDNSZoneList{}
	listOptions := client.InNamespace(h.project)
	if err := h.client.List(context.Background(), managedZoneList, listOptions); err != nil {
		return nil, fmt.Errorf("failed to list ManagedDNSZone in project %q: %v", h.project, err)
	}

	for _, zone := range managedZoneList.Items {
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), h.makeZoneID(zone.Name), dns.NormalizeHostname(zone.Spec.DNSName), zone.Name, false)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

// GetZoneState gets DNSHostedZoneState from cache.
func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, _ provider.ZoneCache) (provider.DNSZoneState, error) {
	// Update metrics of list records call.
	rt := provider.M_LISTRECORDS
	h.config.Metrics.AddZoneRequests(zone.Id().ID, rt, 1)
	h.config.RateLimiter.Accept()

	dnssets := dns.DNSSets{}
	resourceRecordSetList := &globalnetworkingv1.ResourceRecordSetList{}
	err := h.client.List(
		context.Background(),
		resourceRecordSetList,
		client.InNamespace(h.project),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve recordset in project %s: %w", h.project, err)
	}

	for _, rrSet := range resourceRecordSetList.Items {
		var records []*dns.Record
		spec := rrSet.Spec
		recordType := spec.Type
		if spec.DNSZone == zone.Key() && isRecordTypeSupported(recordType) {
			ttl := dnsconst.DNSDefaultTTL
			if spec.TTLSeconds != nil && *spec.TTLSeconds > 0 {
				ttl = int(*spec.TTLSeconds)
			}

			for _, value := range spec.RRData {
				records = append(records, &dns.Record{
					Value: value,
				})
			}
			logger.Infof("DNS Name: %v, Records of type %s: %s", rrSet.Name, recordType, spec.RRData)
			dnssets.AddRecordSetFromProvider(rrSet.Name, dns.NewRecordSet(recordType, int64(ttl), records))
		}
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func isRecordTypeSupported(recordType string) bool {
	for _, supportedRecordType := range supportedRecordTypes {
		if recordType == supportedRecordType {
			return true
		}
	}
	return false
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, _ provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h, zone)

	var succeeded, failed int
	for _, r := range reqs {
		change, err := exec.prepareChange(r)
		if err != nil {
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			logger.Infof("Invalid updates for records in zone %s: %v", zone.Domain(), err)
			continue
		}

		if h.config.DryRun {
			continue
		}

		err = exec.applyChange(change)
		if err != nil {
			failed++
			if r.Done != nil {
				r.Done.Failed(err)
			}
			logger.Infof("Failed updates for records in zone %s: %v", zone.Domain(), err)
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for GDC")
		return nil
	}

	if succeeded > 0 {
		logger.Infof("Summary: succeeded updates for records in zone %s: %d", zone.Domain(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Summary: failed updates for records in zone %s: %d", zone.Domain(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}

func zoneNames(cf *corefile.Corefile) []string {
	seenZones := make(map[string]bool)
	var zoneNames []string
	for _, s := range cf.Servers {
		for _, d := range s.DomPorts {
			// The field can be just the zone, or <zone>:<port>.
			name := strings.Split(d, ":")[0]
			if !seenZones[name] {
				zoneNames = append(zoneNames, name)
				seenZones[name] = true
			}
		}
	}
	sort.Strings(zoneNames)
	return zoneNames
}

func (h *Handler) makeZoneID(zoneName string) string {
	return fmt.Sprintf("%s/%s", h.project, zoneName)
}

// SplitZoneID splits the zone id into project id and zone name
func SplitZoneID(id string) (string, string, error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid zone id: %s", id)
	}
	return parts[0], parts[1], nil
}
