// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnserrors "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

var DNSProviderType = (*api.DNSProvider)(nil)

type DNSProviderObject struct {
	resources.Object
}

func (this *DNSProviderObject) DNSProvider() *api.DNSProvider {
	return this.Data().(*api.DNSProvider)
}

func DNSProviderKey(namespace, name string) resources.ObjectKey {
	return resources.NewKey(schema.GroupKind{Group: api.GroupName, Kind: api.DNSProviderKind}, namespace, name)
}

func (this *DNSProviderObject) Spec() *api.DNSProviderSpec {
	return &this.DNSProvider().Spec
}

func (this *DNSProviderObject) StatusField() any {
	return &this.DNSProvider().Status
}

func (this *DNSProviderObject) Status() *api.DNSProviderStatus {
	return &this.DNSProvider().Status
}

func (this *DNSProviderObject) TypeCode() string {
	return this.DNSProvider().Spec.Type
}

func (this *DNSProviderObject) SetStateWithError(_ string, err error) bool {
	type causer interface {
		error
		Cause() error
	}

	message := err.Error()
	handlerErrorMsg := ""
	for {
		var cause error
		if err := errors.Unwrap(err); err != nil {
			cause = err
		} else if err, ok := err.(causer); ok {
			cause = err.Cause()
		}
		if cause == nil || cause == err {
			break
		}
		if dnserrors.IsHandlerError(cause) {
			handlerErrorMsg = cause.Error()
			break
		}
		err = cause
	}
	if handlerErrorMsg != "" {
		prefix := message
		if len(message) > len(handlerErrorMsg) && strings.HasSuffix(message, handlerErrorMsg) {
			prefix = message[:len(message)-len(handlerErrorMsg)]
		}
		return this.SetState(api.STATE_ERROR, message, prefix)
	}
	return this.SetState(api.STATE_ERROR, message)
}

func (this *DNSProviderObject) SetState(state, message string, commonMessagePrefix ...string) bool {
	mod := &utils.ModificationState{}
	status := &this.DNSProvider().Status
	if len(commonMessagePrefix) != 1 || status.Message == nil || !strings.HasPrefix(*status.Message, commonMessagePrefix[0]) {
		// only update if prefix has changed. This avoids race conditions (state update <-> reconcilie) if message
		// contains time stamps or correlation ids
		mod.AssureStringPtrValue(&status.Message, message)
	}
	mod.AssureStringValue(&status.State, state)
	mod.AssureInt64Value(&status.ObservedGeneration, this.DNSProvider().Generation)
	return mod.IsModified()
}

func (this *DNSProviderObject) SetSelection(included, excluded utils.StringSet, target *api.DNSSelectionStatus) bool {
	modified := false
	old_inc := utils.NewStringSetByArray(target.Included)
	if !old_inc.Equals(included) {
		modified = true
		target.Included = included.AsArray()
	}
	old_exc := utils.NewStringSetByArray(target.Excluded)
	if !old_exc.Equals(excluded) {
		modified = true
		target.Excluded = excluded.AsArray()
	}
	return modified
}

func DNSProvider(o resources.Object) *DNSProviderObject {
	if o.IsA(DNSProviderType) {
		return &DNSProviderObject{o}
	}
	return nil
}
