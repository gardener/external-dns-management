/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

func hasFinalizer(key string, obj ObjectData) bool {
	for _, name := range obj.GetFinalizers() {
		if name == key {
			return true
		}
	}
	return false
}

func (this *AbstractObject) HasFinalizer(key string) bool {
	for _, name := range this.GetFinalizers() {
		if name == key {
			return true
		}
	}
	return false
}

func (this *AbstractObject) SetFinalizer(key string) error {
	if !hasFinalizer(key, this.ObjectData) {
		this.ObjectData.SetFinalizers(append(this.ObjectData.GetFinalizers(), key))
	}
	return nil
}

func (this *AbstractObject) RemoveFinalizer(key string) error {
	list := this.ObjectData.GetFinalizers()
	for i, name := range list {
		if name == key {
			this.ObjectData.SetFinalizers(append(list[:i], list[i+1:]...))
			return nil
		}
	}
	return nil
}
