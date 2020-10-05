/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

func (this *AbstractObject) GetLabel(name string) string {
	labels := this.ObjectData.GetLabels()
	return labels[name]
}
