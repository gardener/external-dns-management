// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"

func isGatewayAPICRD(name string) bool {
	return name == "gateways."+gatewayapisv1.GroupName ||
		name == "httproutes."+gatewayapisv1.GroupName
}
