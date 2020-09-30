/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

////////////////////////////////////////////////////////////////////////////////
// ObjectKey
////////////////////////////////////////////////////////////////////////////////

func EqualsObjectKey(a, b ObjectKey) bool {
	return EqualsObjectName(a.name, b.name) &&
		a.groupKind == b.groupKind
}

var _ GroupKindProvider = ObjectKey{}

func NewKey(groupKind schema.GroupKind, namespace, name string) ObjectKey {
	return ObjectKey{groupKind, NewObjectName(namespace, name)}
}

func NewKeyForData(data ObjectData) ObjectKey {
	return ObjectKey{data.GetObjectKind().GroupVersionKind().GroupKind(), NewObjectNameForData(data)}
}

func (this ObjectKey) GroupKind() schema.GroupKind {
	return this.groupKind
}

func (this ObjectKey) Group() string {
	return this.groupKind.Group
}

func (this ObjectKey) Kind() string {
	return this.groupKind.Kind
}

func (this ObjectKey) Namespace() string {
	return this.name.Namespace()
}

func (this ObjectKey) ObjectName() ObjectName {
	return this.name
}

func (this ObjectKey) Name() string {
	return this.name.Name()
}

func (this ObjectKey) ForCluster(id string) ClusterObjectKey {
	return ClusterObjectKey{id, objectKey{this}}
}

func (this ObjectKey) String() string {
	return fmt.Sprintf("%s/%s/%s", this.groupKind.Group, this.groupKind.Kind, this.name)
}

func NewGroupKind(group, kind string) schema.GroupKind {
	if group == "core" {
		group = ""
	}
	return schema.GroupKind{Group: group, Kind: kind}
}

////////////////////////////////////////////////////////////////////////////////
// ClusterObjectKey
////////////////////////////////////////////////////////////////////////////////

type KeyFilter func(ClusterObjectKey) bool

func EqualsClusterObjectKey(a, b ClusterObjectKey) bool {
	return EqualsObjectName(a.name, b.name) &&
		a.groupKind == b.groupKind &&
		a.cluster == b.cluster
}

func NewClusterKeyForObject(cluster string, key ObjectKey) ClusterObjectKey {
	return ClusterObjectKey{cluster, objectKey{key}}
}

func NewClusterKey(cluster string, groupKind schema.GroupKind, namespace, name string) ClusterObjectKey {
	return ClusterObjectKey{cluster, objectKey{ObjectKey{groupKind, NewObjectName(namespace, name)}}}
}

func (this ClusterObjectKey) ChangeCluster(id string) ClusterObjectKey {
	this.cluster = id
	return this
}

func (this ClusterObjectKey) String() string {
	return this.asString()
}

func (this ClusterObjectKey) asString() string {
	return fmt.Sprintf("%s:%s", this.cluster, this.objectKey.ObjectKey)
}

func (this ClusterObjectKey) Cluster() string {
	return this.cluster
}

func (this ClusterObjectKey) ClusterGroupKind() ClusterGroupKind {
	return NewClusterGroupKind(this.cluster, this.GroupKind())
}

func (this ClusterObjectKey) ObjectKey() ObjectKey {
	return this.objectKey.ObjectKey
}

func (this ClusterObjectKey) AsRefFor(clusterid string) string {
	if this.cluster == clusterid {
		return this.objectKey.String()
	}
	return this.asString()
}

func (a ClusterObjectKey) Compare(b ClusterObjectKey) int {
	if a == b {
		return 0
	}
	r := strings.Compare(a.Cluster(), b.Cluster())
	if r == 0 {
		r = strings.Compare(a.Group(), b.Group())
		if r == 0 {
			r = strings.Compare(a.Kind(), b.Kind())
			if r == 0 {
				r = strings.Compare(a.Namespace(), b.Namespace())
				if r == 0 {
					r = strings.Compare(a.Name(), b.Name())
				}
			}
		}
	}
	return r
}

func ParseClusterObjectKey(clusterid string, key string) (ClusterObjectKey, error) {
	id := clusterid
	i := strings.Index(key, ":")
	if i >= 0 {
		id = key[:i]
		key = key[i+1:]
	}
	comps := strings.Split(key, "/")
	switch len(comps) {
	case 4:
		return NewClusterKey(id, NewGroupKind(comps[0], comps[1]), comps[2], comps[3]), nil
	default:
		return ClusterObjectKey{}, errors.NewInvalid("invalid cluster object key format: %s", key)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Sortable Cluster Object Key Array
////////////////////////////////////////////////////////////////////////////////

type ClusterObjectKeys []ClusterObjectKey

func (p ClusterObjectKeys) Len() int           { return len(p) }
func (p ClusterObjectKeys) Less(i, j int) bool { return p[i].Compare(p[j]) < 0 }
func (p ClusterObjectKeys) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

////////////////////////////////////////////////////////////////////////////////
// Cluster Object Key Set
////////////////////////////////////////////////////////////////////////////////

type ClusterObjectKeySet map[ClusterObjectKey]struct{}

func NewClusterObjectKeySet(a ...ClusterObjectKey) ClusterObjectKeySet {
	return ClusterObjectKeySet{}.Add(a...)
}

func NewClusterObjectKeySetByArray(a []ClusterObjectKey) ClusterObjectKeySet {
	s := ClusterObjectKeySet{}
	if a != nil {
		s.Add(a...)
	}
	return s
}

func NewClusterObjectKeSetBySets(sets ...ClusterObjectKeySet) ClusterObjectKeySet {
	s := ClusterObjectKeySet{}
	for _, set := range sets {
		for a := range set {
			s.Add(a)
		}
	}
	return s
}

func (this ClusterObjectKeySet) String() string {
	sep := ""
	data := "["
	for k := range this {
		data = fmt.Sprintf("%s%s'%s'", data, sep, k)
		sep = ", "
	}
	return data + "]"
}

func (this ClusterObjectKeySet) Contains(n ClusterObjectKey) bool {
	_, ok := this[n]
	return ok
}

func (this ClusterObjectKeySet) Remove(n ClusterObjectKey) ClusterObjectKeySet {
	delete(this, n)
	return this
}

func (this ClusterObjectKeySet) AddAll(n []ClusterObjectKey) ClusterObjectKeySet {
	return this.Add(n...)
}

func (this ClusterObjectKeySet) Add(n ...ClusterObjectKey) ClusterObjectKeySet {
	for _, p := range n {
		this[p] = struct{}{}
	}
	return this
}

func (this ClusterObjectKeySet) AddSet(sets ...ClusterObjectKeySet) ClusterObjectKeySet {
	for _, s := range sets {
		for e := range s {
			this.Add(e)
		}
	}
	return this
}

func (this ClusterObjectKeySet) Equals(set ClusterObjectKeySet) bool {
	for n := range set {
		if !this.Contains(n) {
			return false
		}
	}
	for n := range this {
		if !set.Contains(n) {
			return false
		}
	}
	return true
}

func (this ClusterObjectKeySet) DiffFrom(set ClusterObjectKeySet) (add, del ClusterObjectKeySet) {
	add = ClusterObjectKeySet{}
	del = ClusterObjectKeySet{}
	for n := range set {
		if !this.Contains(n) {
			add.Add(n)
		}
	}
	for n := range this {
		if !set.Contains(n) {
			del.Add(n)
		}
	}
	return
}

func (this ClusterObjectKeySet) Copy() ClusterObjectKeySet {
	set := NewClusterObjectKeySet()
	for n := range this {
		set[n] = struct{}{}
	}
	return set
}

func (this ClusterObjectKeySet) AsArray() ClusterObjectKeys {
	a := []ClusterObjectKey{}
	for n := range this {
		a = append(a, n)
	}
	return a
}

func (this ClusterObjectKeySet) Filter(filter KeyFilter) ClusterObjectKeySet {
	if this == nil {
		return nil
	}
	if filter == nil {
		return this
	}
	set := NewClusterObjectKeySet()
	for n := range this {
		if filter(n) {
			set[n] = struct{}{}
		}
	}
	return set
}

////////////////////////////////////////////////////////////////////////////////
// Group Kind Set
////////////////////////////////////////////////////////////////////////////////

type GroupKindSet map[schema.GroupKind]struct{}

func NewGroupKindSet(a ...schema.GroupKind) GroupKindSet {
	return GroupKindSet{}.Add(a...)
}

func NewGroupKindSetByArray(a []schema.GroupKind) GroupKindSet {
	s := GroupKindSet{}
	if a != nil {
		s.Add(a...)
	}
	return s
}

func NewGroupKindSetBySets(sets ...GroupKindSet) GroupKindSet {
	s := GroupKindSet{}
	for _, set := range sets {
		for a := range set {
			s.Add(a)
		}
	}
	return s
}

func (this GroupKindSet) String() string {
	sep := ""
	data := "["
	for k := range this {
		data = fmt.Sprintf("%s%s'%s'", data, sep, k)
		sep = ", "
	}
	return data + "]"
}

func (this GroupKindSet) Contains(n schema.GroupKind) bool {
	if this == nil {
		return false
	}
	_, ok := this[n]
	return ok
}

func (this GroupKindSet) Remove(n schema.GroupKind) GroupKindSet {
	delete(this, n)
	return this
}

func (this GroupKindSet) AddAll(n []schema.GroupKind) GroupKindSet {
	return this.Add(n...)
}

func (this GroupKindSet) Add(n ...schema.GroupKind) GroupKindSet {
	for _, p := range n {
		this[p] = struct{}{}
	}
	return this
}

func (this GroupKindSet) AddSet(sets ...GroupKindSet) GroupKindSet {
	for _, s := range sets {
		for e := range s {
			this.Add(e)
		}
	}
	return this
}

func (this GroupKindSet) Equals(set GroupKindSet) bool {
	for n := range set {
		if !this.Contains(n) {
			return false
		}
	}
	for n := range this {
		if !set.Contains(n) {
			return false
		}
	}
	return true
}

func (this GroupKindSet) DiffFrom(set GroupKindSet) (add, del GroupKindSet) {
	add = GroupKindSet{}
	del = GroupKindSet{}
	for n := range set {
		if !this.Contains(n) {
			add.Add(n)
		}
	}
	for n := range this {
		if !set.Contains(n) {
			del.Add(n)
		}
	}
	return
}

func (this GroupKindSet) Copy() GroupKindSet {
	set := NewGroupKindSet()
	for n := range this {
		set[n] = struct{}{}
	}
	return set
}

func (this GroupKindSet) AsArray() []schema.GroupKind {
	a := []schema.GroupKind{}
	for n := range this {
		a = append(a, n)
	}
	return a
}

////////////////////////////////////////////////////////////////////////////////
// Cluster Group Kind Set
////////////////////////////////////////////////////////////////////////////////

type ClusterGroupKindSet map[ClusterGroupKind]struct{}

func NewClusterGroupKindSet(a ...ClusterGroupKind) ClusterGroupKindSet {
	return ClusterGroupKindSet{}.Add(a...)
}

func NewClusterGroupKindSetByArray(a []ClusterGroupKind) ClusterGroupKindSet {
	s := ClusterGroupKindSet{}
	if a != nil {
		s.Add(a...)
	}
	return s
}

func NewClusterGroupKindSetBySets(sets ...ClusterGroupKindSet) ClusterGroupKindSet {
	s := ClusterGroupKindSet{}
	for _, set := range sets {
		for a := range set {
			s.Add(a)
		}
	}
	return s
}

func (this ClusterGroupKindSet) String() string {
	sep := ""
	data := "["
	for k := range this {
		data = fmt.Sprintf("%s%s'%s'", data, sep, k)
		sep = ", "
	}
	return data + "]"
}

func (this ClusterGroupKindSet) Contains(n ClusterGroupKind) bool {
	_, ok := this[n]
	return ok
}

func (this ClusterGroupKindSet) Remove(n ClusterGroupKind) ClusterGroupKindSet {
	delete(this, n)
	return this
}

func (this ClusterGroupKindSet) AddAll(n []ClusterGroupKind) ClusterGroupKindSet {
	return this.Add(n...)
}

func (this ClusterGroupKindSet) Add(n ...ClusterGroupKind) ClusterGroupKindSet {
	for _, p := range n {
		this[p] = struct{}{}
	}
	return this
}

func (this ClusterGroupKindSet) AddSet(sets ...ClusterGroupKindSet) ClusterGroupKindSet {
	for _, s := range sets {
		for e := range s {
			this.Add(e)
		}
	}
	return this
}

func (this ClusterGroupKindSet) Equals(set ClusterGroupKindSet) bool {
	for n := range set {
		if !this.Contains(n) {
			return false
		}
	}
	for n := range this {
		if !set.Contains(n) {
			return false
		}
	}
	return true
}

func (this ClusterGroupKindSet) DiffFrom(set ClusterGroupKindSet) (add, del ClusterGroupKindSet) {
	add = ClusterGroupKindSet{}
	del = ClusterGroupKindSet{}
	for n := range set {
		if !this.Contains(n) {
			add.Add(n)
		}
	}
	for n := range this {
		if !set.Contains(n) {
			del.Add(n)
		}
	}
	return
}

func (this ClusterGroupKindSet) Copy() ClusterGroupKindSet {
	set := ClusterGroupKindSet{}
	for n := range this {
		set[n] = struct{}{}
	}
	return set
}

func (this ClusterGroupKindSet) AsArray() []ClusterGroupKind {
	a := []ClusterGroupKind{}
	for n := range this {
		a = append(a, n)
	}
	return a
}

////////////////////////////////////////////////////////////////////////////////
// Object Name
////////////////////////////////////////////////////////////////////////////////

type GenericObjectName interface {
	ObjectName
	ObjectDataName
}

type objectName struct {
	namespace string
	name      string
}

func NewObjectNameFor(p ObjectNameProvider) GenericObjectName {
	if p == nil {
		return nil
	}
	return NewObjectName(p.Namespace(), p.Name())
}

func NewObjectNameForData(p ObjectDataName) GenericObjectName {
	if p == nil {
		return nil
	}
	return NewObjectName(p.GetNamespace(), p.GetName())
}

func NewObjectName(names ...string) GenericObjectName {
	switch len(names) {
	case 1:
		return objectName{"", names[0]}
	case 2:
		return objectName{names[0], names[1]}
	default:
		panic(fmt.Errorf("objectname has one or two arguments (got %d)", len(names)))
	}
}

func (this objectName) GetNamespace() string {
	return this.namespace
}

func (this objectName) GetName() string {
	return this.name
}

func (this objectName) Namespace() string {
	return this.namespace
}

func (this objectName) Name() string {
	return this.name
}

func (this objectName) ForGroupKind(gk schema.GroupKind) ObjectKey {
	return NewKey(gk, this.namespace, this.name)
}

func (this objectName) String() string {
	return fmt.Sprintf("%s/%s", this.namespace, this.name)
}

func ParseObjectName(name string) (GenericObjectName, error) {
	comps := strings.Split(name, "/")
	switch len(comps) {
	case 0:
		return nil, nil
	case 1, 2:
		return NewObjectName(comps...), nil
	default:
		return nil, errors.NewInvalid("illegal object name %q", name)
	}
}

func EqualsObjectName(a, b ObjectName) bool {
	return a.Name() == b.Name() && a.Namespace() == b.Namespace()
}

////////////////////////////////////////////////////////////////////////////////
// Object Name Set
////////////////////////////////////////////////////////////////////////////////

type ObjectNameSet map[ObjectName]struct{}

func NewObjectNameSet(a ...ObjectName) ObjectNameSet {
	return ObjectNameSet{}.Add(a...)
}

func NewObjectNameSetByArray(a []ObjectName) ObjectNameSet {
	s := ObjectNameSet{}
	if a != nil {
		s.Add(a...)
	}
	return s
}

func NewObjectNameSetBySets(sets ...ObjectNameSet) ObjectNameSet {
	s := ObjectNameSet{}
	for _, set := range sets {
		for a := range set {
			s.Add(a)
		}
	}
	return s
}

func (this ObjectNameSet) String() string {
	sep := ""
	data := "["
	for k := range this {
		data = fmt.Sprintf("%s%s'%s'", data, sep, k)
		sep = ", "
	}
	return data + "]"
}

func (this ObjectNameSet) Contains(n ObjectName) bool {
	_, ok := this[n]
	return ok
}

func (this ObjectNameSet) Remove(n ObjectName) ObjectNameSet {
	delete(this, n)
	return this
}

func (this ObjectNameSet) AddAll(n []ObjectName) ObjectNameSet {
	return this.Add(n...)
}

func (this ObjectNameSet) Add(n ...ObjectName) ObjectNameSet {
	for _, p := range n {
		this[p] = struct{}{}
	}
	return this
}

func (this ObjectNameSet) AddSet(sets ...ObjectNameSet) ObjectNameSet {
	for _, s := range sets {
		for e := range s {
			this.Add(e)
		}
	}
	return this
}

func (this ObjectNameSet) AddAllSplitted(n string) (ObjectNameSet, error) {
	for _, p := range strings.Split(n, ",") {
		o, err := ParseObjectName(strings.TrimSpace(p))
		if err != nil {
			return nil, err
		}
		this.Add(o)
	}
	return this, nil
}

func (this ObjectNameSet) Equals(set ObjectNameSet) bool {
	for n := range set {
		if !this.Contains(n) {
			return false
		}
	}
	for n := range this {
		if !set.Contains(n) {
			return false
		}
	}
	return true
}

func (this ObjectNameSet) DiffFrom(set ObjectNameSet) (add, del ObjectNameSet) {
	add = ObjectNameSet{}
	del = ObjectNameSet{}
	for n := range set {
		if !this.Contains(n) {
			add.Add(n)
		}
	}
	for n := range this {
		if !set.Contains(n) {
			del.Add(n)
		}
	}
	return
}

func (this ObjectNameSet) Copy() ObjectNameSet {
	set := NewObjectNameSet()
	for n := range this {
		set[n] = struct{}{}
	}
	return set
}

func (this ObjectNameSet) AsArray() []ObjectName {
	a := []ObjectName{}
	for n := range this {
		a = append(a, n)
	}
	return a
}
