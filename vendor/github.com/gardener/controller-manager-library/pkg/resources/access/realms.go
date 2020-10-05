/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package access

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

/*
 * If krac is NOT used, there would be no useful access control
 * mechanism. Therefore we support some special cases by introducing the
 * notion of realms. An object might be "used" by another one
 * if its set of responsibility realms contains at least one of the
 * realms of the other object.
 */

type RealmTypes map[string]*RealmType

////////////////////////////////////////////////////////////////////////////////

type RealmType struct {
	annotation string
}

func NewRealmType(anno string) *RealmType {
	return &RealmType{anno}
}

func (this *RealmType) GetAnnotation() string {
	return this.annotation
}

func (this *RealmType) RealmsForObject(obj metav1.Object) *Realms {
	realm := obj.GetAnnotations()[this.annotation]
	return this.NewRealms(realm)
}

func (this *RealmType) NewRealms(value string) *Realms {
	c := &Realms{realms: utils.StringSet{}, rtype: this}
	if value == "" {
		c.realms.Add(value)
	} else {
		c.realms.AddAllSplitted(value)
	}
	return c
}

////////////////////////////////////////////////////////////////////////////////

type Realms struct {
	realms utils.StringSet
	rtype  *RealmType
}

func (this *Realms) String() string {
	return this.realms.String()
}

func (this *Realms) AnnotationValue() string {
	sep := ""
	data := ""
	for k := range this.realms {
		data = fmt.Sprintf("%s%s%s", data, sep, k)
		sep = ","
	}
	return data
}

func (this *Realms) IsDefault() bool {
	return this.Size() == 0 || (this.Size() == 1 && this.Contains(""))
}

func (this *Realms) Size() int {
	return len(this.realms)
}

func (this *Realms) Realms() utils.StringSet {
	return this.realms.Copy()
}

func (this *Realms) Contains(realm string) bool {
	return this.realms.Contains(realm)
}

func (this *Realms) ContainsAnyOf(realms *Realms) bool {
	for r := range this.realms {
		if realms.Contains(r) {
			return true
		}
	}
	return false
}

func (this *Realms) IsResponsibleFor(obj resources.Object) bool {
	if this.ContainsAnyOf(this.rtype.RealmsForObject(obj)) {
		return true
	}
	return false
}
