// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LastOperationTimestamp returns the timestamp for a LastOperation update. If the old operation is nil or any field has changed, it returns the current time; otherwise, it returns the previous update time.
func LastOperationTimestamp(oldOperation *gardencorev1beta1.LastOperation, newOperation gardencorev1beta1.LastOperation) metav1.Time {
	if oldOperation == nil {
		return metav1.Now()
	}
	if oldOperation.State != newOperation.State ||
		oldOperation.Type != newOperation.Type ||
		oldOperation.Description != newOperation.Description ||
		oldOperation.Progress != newOperation.Progress {
		return metav1.Now()
	}
	return oldOperation.LastUpdateTime
}
