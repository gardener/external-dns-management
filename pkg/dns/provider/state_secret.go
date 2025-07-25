// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

////////////////////////////////////////////////////////////////////////////////
// state handling for secrets
////////////////////////////////////////////////////////////////////////////////

func (this *state) UpdateSecret(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if this.config.RemoteAccessConfig != nil &&
		obj.ObjectName().Name() == this.config.RemoteAccessConfig.SecretName.Name() &&
		obj.ObjectName().Namespace() == this.config.RemoteAccessConfig.SecretName.Namespace() {
		fullObj, err := obj.GetFullObject()
		if err != nil {
			return reconcile.Failed(logger, err)
		}
		secret := fullObj.Data().(*corev1.Secret)
		this.config.RemoteAccessConfig.ServerSecretProvider.UpdateSecret(secret)
	}

	providers := this.GetSecretUsage(obj.ObjectName())
	if len(providers) == 0 {
		return reconcile.DelayOnError(logger, this.RemoveFinalizer(obj))
	}
	logger.Infof("reconcile SECRET")
	for _, p := range providers {
		logger.Infof("requeueing provider %q using secret %q", p.ObjectName(), obj.ObjectName())
		if err := this.context.Enqueue(p); err != nil {
			panic(fmt.Sprintf("cannot enqueue provider %q: %s", p.Description(), err))
		}
	}
	return reconcile.Succeeded(logger)
}

func (this *state) GetSecretUsage(name resources.ObjectName) []resources.Object {
	this.lock.RLock()
	defer this.lock.RUnlock()

	set := this.secrets[name]
	if set == nil {
		return []resources.Object{}
	}
	result := make([]resources.Object, 0, len(set))
	for n := range set {
		o := this.providers[n]
		if o != nil {
			result = append(result, o.object)
		}
	}
	return result
}

func (this *state) registerSecret(logger logger.LogContext, secret resources.ObjectName, provider *dnsProviderVersion) (bool, error) {
	pname := provider.ObjectName()
	old := this.providersecrets[pname]

	if old != nil && old != secret {
		oldp := this.secrets[old]
		if oldp.Contains(pname) {
			logger.Infof("releasing secret %q for provider %q", old, pname)
			if len(oldp) <= 1 {
				r, err := provider.Object().Resources().Get(&corev1.Secret{})
				if err != nil {
					logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
					return true, err
				}
				s, err := r.Get(old)
				if err != nil {
					if !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return true, err
					}
				} else {
					logger.Infof("remove finalizer for unused secret %q", old)
					err := this.RemoveFinalizer(s)
					if err != nil && !errors.IsNotFound(err) {
						logger.Warnf("cannot release secret %q for provider %q: %s", old, pname, err)
						return true, err
					}
				}
				delete(this.secrets, old)
			} else {
				delete(oldp, pname)
			}
		}
	}
	mod := false
	if secret != nil {
		if old != secret {
			logger.Infof("registering secret %q for provider %q", secret, pname)
			this.providersecrets[pname] = secret

			curp := this.secrets[secret]
			if curp == nil {
				curp = resources.ObjectNameSet{}
				this.secrets[secret] = curp
			}
			curp.Add(pname)
			mod = true
		}

		r, _ := provider.Object().Resources().Get(&corev1.Secret{})
		s, err := r.Get(secret)
		if err == nil {
			err = this.SetFinalizer(s)
		}
		if err != nil {
			if errors.IsNotFound(err) {
				return mod, fmt.Errorf("secret %q for provider %q not found", secret, pname)
			} else {
				return mod, fmt.Errorf("cannot set finalizer for secret %q for provider %q: %s", secret, pname, err)
			}
		}
	} else {
		delete(this.providersecrets, pname)
	}
	return mod, nil
}
