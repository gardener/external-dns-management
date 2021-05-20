/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package v1_16 // golint: ignore

import (
	admissionregistration "k8s.io/api/admissionregistration/v1"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	networking "k8s.io/api/networking/v1beta1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	. "github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

func init() {
	Register(core.SchemeBuilder)
	Register(extensions.SchemeBuilder)
	Register(apps.SchemeBuilder)
	Register(admissionregistration.SchemeBuilder)
	Register(apiextensions.SchemeBuilder)
	Register(networking.SchemeBuilder)
}
