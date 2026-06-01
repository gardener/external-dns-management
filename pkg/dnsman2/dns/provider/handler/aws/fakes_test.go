// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// fakeRoute53 is a controllable fake of the route53API interface used by the AWS handler.
// Each function field can be set per test; unset functions return an error so unexpected
// calls are surfaced.
type fakeRoute53 struct {
	listHostedZonesFn       func(context.Context, *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
	listResourceRecordsFn   func(context.Context, *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error)
	changeResourceRecordsFn func(context.Context, *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	getHostedZoneFn         func(context.Context, *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error)

	changeCalls []*route53.ChangeResourceRecordSetsInput
}

func (f *fakeRoute53) ListHostedZones(ctx context.Context, params *route53.ListHostedZonesInput, _ ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error) {
	if f.listHostedZonesFn == nil {
		return nil, fmt.Errorf("ListHostedZones not stubbed")
	}
	return f.listHostedZonesFn(ctx, params)
}

func (f *fakeRoute53) ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	if f.listResourceRecordsFn == nil {
		return nil, fmt.Errorf("ListResourceRecordSets not stubbed")
	}
	return f.listResourceRecordsFn(ctx, params)
}

func (f *fakeRoute53) ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	f.changeCalls = append(f.changeCalls, params)
	if f.changeResourceRecordsFn == nil {
		return &route53.ChangeResourceRecordSetsOutput{}, nil
	}
	return f.changeResourceRecordsFn(ctx, params)
}

func (f *fakeRoute53) GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, _ ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error) {
	if f.getHostedZoneFn == nil {
		return nil, fmt.Errorf("GetHostedZone not stubbed")
	}
	return f.getHostedZoneFn(ctx, params)
}

func (f *fakeRoute53) AssociateVPCWithHostedZone(_ context.Context, _ *route53.AssociateVPCWithHostedZoneInput, _ ...func(*route53.Options)) (*route53.AssociateVPCWithHostedZoneOutput, error) {
	return nil, fmt.Errorf("AssociateVPCWithHostedZone not stubbed")
}

func (f *fakeRoute53) DisassociateVPCFromHostedZone(_ context.Context, _ *route53.DisassociateVPCFromHostedZoneInput, _ ...func(*route53.Options)) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
	return nil, fmt.Errorf("DisassociateVPCFromHostedZone not stubbed")
}

func (f *fakeRoute53) CreateVPCAssociationAuthorization(_ context.Context, _ *route53.CreateVPCAssociationAuthorizationInput, _ ...func(*route53.Options)) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
	return nil, fmt.Errorf("CreateVPCAssociationAuthorization not stubbed")
}

func (f *fakeRoute53) DeleteVPCAssociationAuthorization(_ context.Context, _ *route53.DeleteVPCAssociationAuthorizationInput, _ ...func(*route53.Options)) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
	return nil, fmt.Errorf("DeleteVPCAssociationAuthorization not stubbed")
}

func (f *fakeRoute53) ListGeoLocations(_ context.Context, _ *route53.ListGeoLocationsInput, _ ...func(*route53.Options)) (*route53.ListGeoLocationsOutput, error) {
	return &route53.ListGeoLocationsOutput{}, nil
}

func (f *fakeRoute53) ListCidrCollections(_ context.Context, _ *route53.ListCidrCollectionsInput, _ ...func(*route53.Options)) (*route53.ListCidrCollectionsOutput, error) {
	return &route53.ListCidrCollectionsOutput{}, nil
}

func (f *fakeRoute53) ListCidrBlocks(_ context.Context, _ *route53.ListCidrBlocksInput, _ ...func(*route53.Options)) (*route53.ListCidrBlocksOutput, error) {
	return &route53.ListCidrBlocksOutput{}, nil
}

// noopMetrics is a provider.Metrics implementation that records nothing.
// It exists so handler tests don't need a real metrics registry.
type noopMetrics struct{}

func (noopMetrics) AddGenericRequests(_ provider.MetricsRequestType, _ int)        {}
func (noopMetrics) AddZoneRequests(_ string, _ provider.MetricsRequestType, _ int) {}

// newTestHandler builds a *handler wired with the supplied fake route53API.
// Production-only fields like awsConfig.BatchSize are populated with sensible defaults.
func newTestHandler(r53 route53API, blockedZones ...string) *handler {
	return newTestHandlerWith(r53, noopMetrics{}, flowcontrol.NewFakeAlwaysRateLimiter(), blockedZones...)
}

// newTestHandlerWith is like newTestHandler but allows the test to inject custom
// metrics and rate limiter implementations (e.g. counting variants).
func newTestHandlerWith(r53 route53API, metrics provider.Metrics, rateLimiter flowcontrol.RateLimiter, blockedZones ...string) *handler {
	cfg := provider.DNSHandlerConfig{
		Log: logr.Discard(),
		GlobalConfig: config.DNSManagerConfiguration{
			ProviderAdvancedOptions: map[string]config.AdvancedOptions{
				ProviderType: {BlockedZones: blockedZones},
			},
		},
		Metrics:     metrics,
		RateLimiter: rateLimiter,
	}
	return &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            cfg,
		awsConfig:         AWSConfig{BatchSize: defaultBatchSize},
		r53:               r53,
		policyContext:     newRoutingPolicyContext(r53),
	}
}
