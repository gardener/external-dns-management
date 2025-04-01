/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

//go:generate sh -c "CONTROLLER_GEN=$CONTROLLER_GEN bash $GARDENER_HACK_DIR/generate-crds.sh --custom-package 'dns.gardener.cloud=github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1' -p 'crd-' dns.gardener.cloud"

package dns
