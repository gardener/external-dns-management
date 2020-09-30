/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package plain

import (
	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type ModificationState struct {
	*abstract.ModificationState
}

func NewModificationState(object Object, settings ...interface{}) *ModificationState {
	return &ModificationState{abstract.NewModificationState(object, settings...)}
}

func (this *ModificationState) Object() Object {
	return this.ModificationState.Object().(Object)
}

func (this *ModificationState) AssureLabel(name, value string) *ModificationState {
	this.ModificationState.AssureLabel(name, value)
	return this
}
