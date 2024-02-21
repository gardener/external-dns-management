// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate sh -c "CONTROLLER_GEN=$CONTROLLER_GEN bash $CONTROLLER_MANAGER_LIB_HACK_DIR/generate-crds"
// +kubebuilder:skip

package dns

const (
	GroupName = "dns.gardener.cloud"
)
