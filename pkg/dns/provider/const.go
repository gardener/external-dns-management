// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

/*
  Standard options a DNS Reconciler should offer
*/

const (
	OPT_CLASS                      = source.OPT_CLASS
	OPT_DRYRUN                     = "dry-run"
	OPT_TTL                        = "ttl"
	OPT_CACHE_TTL                  = "cache-ttl"
	OPT_SETUP                      = dns.OPT_SETUP
	OPT_DNSDELAY                   = "dns-delay"
	OPT_RESCHEDULEDELAY            = "reschedule-delay"
	OPT_LOCKSTATUSCHECKPERIOD      = "lock-status-check-period"
	OPT_DISABLE_ZONE_STATE_CACHING = "disable-zone-state-caching"
	OPT_DISABLE_DNSNAME_VALIDATION = "disable-dnsname-validation"

	OPT_MAX_METADATA_RECORD_DELETIONS_PER_RECONCILIATION = "max-metadata-record-deletions-per-reconciliation"

	OPT_REMOTE_ACCESS_PORT               = "remote-access-port"
	OPT_REMOTE_ACCESS_CACERT             = "remote-access-cacert"
	OPT_REMOTE_ACCESS_SERVER_SECRET_NAME = "remote-access-server-secret-name"
	OPT_REMOTE_ACCESS_CLIENT_ID          = "remote-access-client-id"

	OPT_PROVIDERTYPES = "provider-types"

	OPT_RATELIMITER_ENABLED = "ratelimiter.enabled"
	OPT_RATELIMITER_QPS     = "ratelimiter.qps"
	OPT_RATELIMITER_BURST   = "ratelimiter.burst"

	OPT_ADVANCED_BATCH_SIZE   = "advanced.batch-size"
	OPT_ADVANCED_MAX_RETRIES  = "advanced.max-retries"
	OPT_ADVANCED_BLOCKED_ZONE = "blocked-zone"

	CMD_HOSTEDZONE_PREFIX = "hostedzone:"

	MSG_THROTTLING = "provider throttled"
)

const (
	AnnotationRemoteAccess = dns.ANNOTATION_GROUP + "/remote-access"
)
