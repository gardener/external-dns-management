// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"
	"reflect"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
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

func (this *DNSProviderObject) SetStateWithError(_ string, err error, errorCodes ...gardencorev1beta1.ErrorCode) bool {
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
		return this.SetState(api.STATE_ERROR, message, errorCodes, prefix)
	}
	return this.SetState(api.STATE_ERROR, message, errorCodes)
}

func (this *DNSProviderObject) SetState(state, message string, errorCodes []gardencorev1beta1.ErrorCode, commonMessagePrefix ...string) bool {
	mod := &utils.ModificationState{}
	status := &this.DNSProvider().Status
	if len(commonMessagePrefix) != 1 || status.Message == nil || !strings.HasPrefix(*status.Message, commonMessagePrefix[0]) {
		// only update if prefix has changed. This avoids race conditions (state update <-> reconcile) if message
		// contains time stamps or correlation ids
		mod.AssureStringPtrValue(&status.Message, message)
	}
	mod.AssureStringValue(&status.State, state)
	this.updateLastOperationAndLastError(mod, message, errorCodes...)
	mod.AssureInt64Value(&status.ObservedGeneration, this.DNSProvider().Generation)
	return mod.IsModified()
}

func (this *DNSProviderObject) updateLastOperationAndLastError(mod *utils.ModificationState, message string, errorCodes ...gardencorev1beta1.ErrorCode) {
	status := &this.DNSProvider().Status
	if len(errorCodes) == 0 {
		errorCodes = DetermineErrorCodes(errors.New(message))
	}
	newLastError := &gardencorev1beta1.LastError{
		Description: message,
		Codes:       errorCodes,
	}

	// Set LastOperation to Failed/Error state
	operationType := gardencorev1beta1.LastOperationTypeReconcile
	if this.DNSProvider().DeletionTimestamp != nil {
		operationType = gardencorev1beta1.LastOperationTypeDelete
	}

	operationState := gardencorev1beta1.LastOperationStateError
	// Use Failed state for non-retryable errors
	if HasNonRetryableErrorCode(errorCodes) {
		operationState = gardencorev1beta1.LastOperationStateFailed
	}

	newLastOperation := &gardencorev1beta1.LastOperation{
		Description: message,
		Progress:    0,
		State:       operationState,
		Type:        operationType,
	}
	b := SetLastOperationAndError(status, newLastOperation, newLastError)
	mod.Modified = mod.Modified || b
	if mod.Modified {
		SetLastUpdateTime(&status.LastUpdateTime)
		SetLastOperationAndErrorTime(status)
	}
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

func SetLastOperationAndError(status *api.DNSProviderStatus, lastOperation *gardencorev1beta1.LastOperation, lastError *gardencorev1beta1.LastError) bool {
	var modified = status.LastOperation == nil ||
		status.LastOperation.State != lastOperation.State ||
		status.LastOperation.Type != lastOperation.Type ||
		status.LastOperation.Description != lastOperation.Description ||
		status.LastOperation.Progress != lastOperation.Progress

	status.LastOperation = lastOperation

	modified = modified || !reflect.DeepEqual(status.LastError, lastError)
	status.LastError = lastError

	return modified
}

func SetLastOperationAndErrorTime(status *api.DNSProviderStatus) {
	if status.LastOperation != nil && status.LastUpdateTime != nil {
		status.LastOperation.LastUpdateTime = *status.LastUpdateTime
	}
	if status.LastError != nil {
		status.LastError.LastUpdateTime = status.LastUpdateTime
	}
}
