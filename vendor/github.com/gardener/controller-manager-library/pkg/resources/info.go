/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"
)

type Info struct {
	groupVersion *schema.GroupVersion
	kind         string
	resourcename string
	namespaced   bool
	subresources utils.StringSet
}

func (this *Info) GroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: this.groupVersion.Group, Version: this.groupVersion.Version, Kind: this.kind}
}

func (this *Info) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: this.groupVersion.Group, Kind: this.kind}
}

func (this *Info) GroupVersion() schema.GroupVersion {
	return schema.GroupVersion{Group: this.groupVersion.Group, Version: this.groupVersion.Version}
}

func (this *Info) GroupVersionResource() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: this.groupVersion.Group, Version: this.groupVersion.Version, Resource: this.resourcename}
}

func (this *Info) Group() string {
	return this.groupVersion.Group
}
func (this *Info) Version() string {
	return this.groupVersion.Version
}

func (this *Info) Kind() string {
	return this.kind
}
func (this *Info) Name() string {
	return this.resourcename
}
func (this *Info) Namespaced() bool {
	return this.namespaced
}
func (this *Info) SubResources() utils.StringSet {
	return utils.NewStringSetBySets(this.subresources)
}
func (this *Info) HasSubResource(name string) bool {
	return this.subresources.Contains(name)
}
func (this *Info) HasStatusSubResource() bool {
	return this.HasSubResource("status")
}

func (this *Info) String() string {
	return fmt.Sprintf("%s/%s %s %s %t", this.groupVersion.Group, this.groupVersion.Version, this.resourcename, this.kind, this.namespaced)
}

func (this *Info) InfoString() string {
	return fmt.Sprintf("%s %s %t", this.resourcename, this.kind, this.namespaced)
}

type ResourceInfos struct {
	lock              sync.RWMutex
	wantedGroups      utils.StringSet
	groupVersionKinds map[schema.GroupVersion]map[string]*Info
	preferredVersions map[schema.GroupKind]string
	cluster           Cluster
	mapper            meta.RESTMapper
	version           *semver.Version
}

func NewResourceInfos(c Cluster, groups utils.StringSet) (*ResourceInfos, error) {
	res := &ResourceInfos{
		wantedGroups:      groups,
		groupVersionKinds: map[schema.GroupVersion]map[string]*Info{},
		preferredVersions: map[schema.GroupKind]string{},
		cluster:           c,
	}
	err := res.update(groups.Contains)
	return res, err
}

func (this *ResourceInfos) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	m, err := this.mapper.RESTMapping(gk, versions...)
	if err != nil {
		err = this.updateRestMapper()
		if err != nil {
			return nil, err
		}
		m, err = this.mapper.RESTMapping(gk, versions...)
	}
	return m, err
}

func (this *ResourceInfos) updateRestMapper() error {
	cfg := this.cluster.Config()
	dc, err := discovery.NewDiscoveryClientForConfig(&cfg)
	if err != nil {
		return err
	}
	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return err
	}
	this.mapper = restmapper.NewDiscoveryRESTMapper(gr)
	return nil
}

func (this *ResourceInfos) updateForGroup(groupName string) error {
	return this.update(func(group string) bool { return group == groupName })
}

func (this *ResourceInfos) update(includeGroup func(group string) bool) error {
	cfg := this.cluster.Config()
	dc, err := discovery.NewDiscoveryClientForConfig(&cfg)
	if err != nil {
		logger.Warnf("failed to get discovery client for cluster %s: %s", this.cluster.GetName(), err)
		return err
	}

	v, err := dc.ServerVersion()
	if err == nil {
		this.version, err = semver.NewVersion(v.GitVersion)
	}
	if err != nil {
		logger.Warnf("failed to get server version for cluster %s (this might be an unexpected response from the kube-apiserver): %s", this.cluster.GetName(), err)
		return err
	}

	groups, err := dc.ServerGroups()
	if err != nil {
		logger.Warnf("failed to get server groups for cluster %s (this might be an unexpected response from the kube-apiserver): %s", this.cluster.GetName(), err)
		return err
	}
	var lastErr error
	total := 0
	for _, group := range groups.Groups {
		if includeGroup(group.Name) {
			count, err := this.doUpdateGroup(dc, group)
			total += count
			if err != nil {
				lastErr = err
				logger.Warnf("failed to get all server resources for group %s of cluster %s: %s", group.Name, this.cluster.GetName(), err)
			}
		}
	}
	if lastErr != nil {
		if total == 0 {
			return lastErr
		}
		logger.Infof("found %d resources", total)
	}

	return nil
}

func (this *ResourceInfos) doUpdateGroup(dc *discovery.DiscoveryClient, group v1.APIGroup) (int, error) {
	var lastErr error
	count := 0
	for _, version := range group.Versions {
		list, err := dc.ServerResourcesForGroupVersion(version.GroupVersion)
		if err != nil {
			lastErr = err
		}
		if list != nil {
			count += len(list.APIResources)
		}

		func() {
			this.lock.Lock()
			defer this.lock.Unlock()
			gv := schema.GroupVersion{Group: group.Name, Version: version.Version}
			m := this.groupVersionKinds[gv]
			if m == nil {
				m = map[string]*Info{}
				this.groupVersionKinds[gv] = m
			}
			for _, r := range list.APIResources {
				if strings.Index(r.Name, "/") < 0 {
					m[r.Kind] = &Info{groupVersion: &gv, resourcename: r.Name, kind: r.Kind, namespaced: r.Namespaced, subresources: utils.StringSet{}}
				}
			}
			for _, r := range list.APIResources {
				if i := strings.Index(r.Name, "/"); i > 0 {
					info := m[r.Kind]
					if info != nil {
						info.subresources.Add(r.Name[i+1:])
					}
				}
			}

			for _, r := range list.APIResources {
				gk := NewGroupKind(gv.Group, r.Kind)
				if _, ok := this.preferredVersions[gk]; !ok || version.Version == group.PreferredVersion.Version {
					this.preferredVersions[gk] = version.Version
				}
			}
		}()
	}
	return count, lastErr
}

func (this *ResourceInfos) GetGroups() []schema.GroupVersion {
	this.lock.RLock()
	defer this.lock.RUnlock()
	grps := []schema.GroupVersion{}
outer:
	for gk, v := range this.preferredVersions {
		gv := schema.GroupVersion{gk.Group, v}
		for _, f := range grps {
			if f.Group == gv.Group && f.Version == gv.Version {
				continue outer
			}
		}
		grps = append(grps, gv)
	}
	return grps
}

func (this *ResourceInfos) GetResourceInfos(gv schema.GroupVersion) []*Info {
	this.lock.RLock()
	defer this.lock.RUnlock()
	m := this.groupVersionKinds[gv]
	if m == nil {
		return []*Info{}
	}
	r := make([]*Info, len(m))[0:0]
	for _, i := range m {
		r = append(r, i)
	}
	return r
}

func (this *ResourceInfos) GetPreferred(gk schema.GroupKind) (*Info, error) {
	i := this.getPreferred(gk)
	if i == nil {
		err := this.updateForGroup(gk.Group)
		if err != nil {
			return nil, err
		}
		i = this.getPreferred(gk)
	}
	if i == nil {
		return nil, errors.ErrUnknownResource.New("group kind", gk)
	}
	return i, nil
}

func (this *ResourceInfos) getPreferred(gk schema.GroupKind) *Info {
	this.lock.RLock()
	defer this.lock.RUnlock()
	v, ok := this.preferredVersions[gk]
	if !ok {
		return nil
	}
	g := this.groupVersionKinds[schema.GroupVersion{Group: gk.Group, Version: v}]
	if g == nil {
		return nil
	}
	return g[gk.Kind]
}

func (this *ResourceInfos) Get(gvk schema.GroupVersionKind) (*Info, error) {
	i := this.get(gvk)
	if i == nil {
		err := this.updateForGroup(gvk.Group)
		if err != nil {
			return nil, err
		}
		i = this.get(gvk)
	}
	if i == nil {
		return nil, errors.ErrUnknownResource.New("group version kind", gvk)
	}
	return i, nil
}

func (this *ResourceInfos) get(gvk schema.GroupVersionKind) *Info {
	this.lock.RLock()
	defer this.lock.RUnlock()
	g := this.groupVersionKinds[gvk.GroupVersion()]
	if g == nil {
		return nil
	}
	return g[gvk.Kind]
}

func (this *ResourceInfos) GetServerVersion() *semver.Version {
	return this.version
}
