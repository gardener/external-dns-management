/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package plain

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type ResourceContext interface {
	abstract.ResourceContext
	Resources() Resources
}

type resourceContext struct {
	*abstract.AbstractResourceContext
}

var _ ResourceContext = &resourceContext{}

func NewResourceContext(ctx context.Context, scheme *runtime.Scheme) ResourceContext {
	res := &resourceContext{}
	res.AbstractResourceContext = abstract.NewAbstractResourceContext(ctx, res, scheme, factory{})
	return res
}

func (this *resourceContext) Resources() Resources {
	return this.AbstractResourceContext.Resources().(Resources)
}
