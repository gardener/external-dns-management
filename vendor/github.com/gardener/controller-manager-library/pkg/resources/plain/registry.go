/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package plain

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

func DeclareDefaultVersion(gv schema.GroupVersion) {
	abstract.DeclareDefaultVersion(gv)
}

func DefaultVersion(g string) string {
	return abstract.DefaultVersion(g)
}

func Register(builders ...runtime.SchemeBuilder) {
	abstract.Register(builders...)
}

func DefaultScheme() *runtime.Scheme {
	return abstract.DefaultScheme()
}
