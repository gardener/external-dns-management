// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate ../../hack/generate-controller-registration.sh dns-external ../../charts/external-dns-management/ ../../VERSION ../../examples/controller-registration.yaml         DNSProvider:aws-route53 DNSProvider:alicloud-dns DNSProvider:azure-dns DNSProvider:azure-private-dns DNSProvider:google-clouddns DNSProvider:openstack-designate DNSProvider:cloudflare-dns DNSProvider:netlify-dns DNSProvider:infoblox-dns DNSProvider:remote

// Package chart enables go:generate support for generating the correct controller registration.
package chart
