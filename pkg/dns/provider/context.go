// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type ProviderContext interface {
	logger.LogContext

	GetContext() context.Context

	IsReady() bool
	GetByExample(runtime.Object) (resources.Interface, error)

	GetStringOption(name string) (string, error)
	GetIntOption(name string) (int, error)

	Synchronize(log logger.LogContext, name string, initiator resources.Object) (bool, error)

	Enqueue(obj resources.Object) error
	EnqueueCommand(cmd string) error
	EnqueueKey(key resources.ClusterObjectKey) error

	SetFinalizer(resources.Object) error
	RemoveFinalizer(resources.Object) error
	HasFinalizer(resources.Object) bool

	GetSecretPropertiesByRef(src resources.ResourcesSource, ref *corev1.SecretReference) (utils.Properties, *resources.SecretObject, error)
	GetPoolPeriod(name string) *time.Duration

	GetCluster(name string) resources.Cluster

	GetPool(name string) controller.Pool
}

type DefaultContext struct {
	logger.LogContext
	controller controller.Interface
}

var _ ProviderContext = &DefaultContext{}

func NewDefaultContext(controller controller.Interface) ProviderContext {
	return &DefaultContext{LogContext: controller, controller: controller}
}

func (this *DefaultContext) GetContext() context.Context {
	return this.controller.GetContext()
}

func (this *DefaultContext) IsReady() bool {
	return this.controller.IsReady()
}

func (this *DefaultContext) GetByExample(obj runtime.Object) (resources.Interface, error) {
	return this.controller.GetMainCluster().Resources().GetByExample(obj)
}

func (this *DefaultContext) GetIntOption(name string) (int, error) {
	return this.controller.GetIntOption(name)
}

func (this *DefaultContext) GetStringOption(name string) (string, error) {
	return this.controller.GetStringOption(name)
}

func (this *DefaultContext) Synchronize(log logger.LogContext, name string, obj resources.Object) (bool, error) {
	return this.controller.Synchronize(log, name, obj)
}

func (this *DefaultContext) Enqueue(obj resources.Object) error {
	return this.controller.Enqueue(obj)
}

func (this *DefaultContext) EnqueueCommand(cmd string) error {
	return this.controller.EnqueueCommand(cmd)
}

func (this *DefaultContext) EnqueueKey(key resources.ClusterObjectKey) error {
	return this.controller.EnqueueKey(key)
}

func (this *DefaultContext) HasFinalizer(obj resources.Object) bool {
	return this.controller.HasFinalizer(obj)
}

func (this *DefaultContext) SetFinalizer(obj resources.Object) error {
	return this.controller.SetFinalizer(obj)
}

func (this *DefaultContext) RemoveFinalizer(obj resources.Object) error {
	return this.controller.RemoveFinalizer(obj)
}

func (this *DefaultContext) GetSecretPropertiesByRef(src resources.ResourcesSource, ref *corev1.SecretReference) (utils.Properties, *resources.SecretObject, error) {
	return resources.GetSecretPropertiesByRef(src, ref)
}

func (this *DefaultContext) GetPoolPeriod(name string) *time.Duration {
	p := this.controller.GetPool(name)
	if p == nil {
		return nil
	}
	d := p.Period()
	return &d
}

func (this *DefaultContext) GetCluster(name string) resources.Cluster {
	return this.controller.GetCluster(name)
}

func (this *DefaultContext) GetPool(name string) controller.Pool {
	return this.controller.GetPool(name)
}
