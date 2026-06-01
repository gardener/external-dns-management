// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/route53"
)

// route53API is the narrow subset of the AWS Route53 client used by this package.
// It exists so tests can substitute a fake without depending on the full SDK client.
type route53API interface {
	ListHostedZones(ctx context.Context, params *route53.ListHostedZonesInput, optFns ...func(*route53.Options)) (*route53.ListHostedZonesOutput, error)
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	GetHostedZone(ctx context.Context, params *route53.GetHostedZoneInput, optFns ...func(*route53.Options)) (*route53.GetHostedZoneOutput, error)
	AssociateVPCWithHostedZone(ctx context.Context, params *route53.AssociateVPCWithHostedZoneInput, optFns ...func(*route53.Options)) (*route53.AssociateVPCWithHostedZoneOutput, error)
	DisassociateVPCFromHostedZone(ctx context.Context, params *route53.DisassociateVPCFromHostedZoneInput, optFns ...func(*route53.Options)) (*route53.DisassociateVPCFromHostedZoneOutput, error)
	CreateVPCAssociationAuthorization(ctx context.Context, params *route53.CreateVPCAssociationAuthorizationInput, optFns ...func(*route53.Options)) (*route53.CreateVPCAssociationAuthorizationOutput, error)
	DeleteVPCAssociationAuthorization(ctx context.Context, params *route53.DeleteVPCAssociationAuthorizationInput, optFns ...func(*route53.Options)) (*route53.DeleteVPCAssociationAuthorizationOutput, error)
	ListGeoLocations(ctx context.Context, params *route53.ListGeoLocationsInput, optFns ...func(*route53.Options)) (*route53.ListGeoLocationsOutput, error)
	ListCidrCollections(ctx context.Context, params *route53.ListCidrCollectionsInput, optFns ...func(*route53.Options)) (*route53.ListCidrCollectionsOutput, error)
	ListCidrBlocks(ctx context.Context, params *route53.ListCidrBlocksInput, optFns ...func(*route53.Options)) (*route53.ListCidrBlocksOutput, error)
}
