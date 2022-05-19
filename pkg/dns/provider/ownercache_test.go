/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package provider

import (
	"context"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ident = "TEST"

////////////////////////////////////////////////////////////////////////////////
// test driver

type owner struct {
	id     string
	valid  *metav1.Time
	active bool
}
type TestOwnerCacheContext struct {
	ids map[OwnerName]owner
	*OwnerCache
}

func (this *TestOwnerCacheContext) GetContext() context.Context {
	return context.Background()
}
func (this *TestOwnerCacheContext) Infof(msg string, args ...interface{}) {
}
func (this *TestOwnerCacheContext) EnqueueKey(key resources.ClusterObjectKey) error {
	cachekey := OwnerName(key.Name())
	old, ok := this.ids[cachekey]
	if ok {
		active := old.active && old.valid.After(time.Now())
		this.OwnerCache.updateOwnerData(cachekey, key, old.id, active, nil, old.valid)
	}
	return nil
}

func (this *TestOwnerCacheContext) updateOwnerData(cachekey OwnerName, id string, active bool) (changeset utils.StringSet, activeset utils.StringSet) {
	return this._updateOwnerData(cachekey, id, active, nil)
}

func (this *TestOwnerCacheContext) updateOwnerDataExpiration(cachekey OwnerName, id string, active bool, valid time.Duration) (changeset utils.StringSet, activeset utils.StringSet) {
	t := metav1.NewTime(time.Now().Add(valid))
	return this._updateOwnerData(cachekey, id, active, &t)
}

func (this *TestOwnerCacheContext) _updateOwnerData(cachekey OwnerName, id string, active bool, valid *metav1.Time) (changeset utils.StringSet, activeset utils.StringSet) {
	this.ids[cachekey] = owner{
		id, valid, active,
	}
	key := resources.NewClusterKey("", schema.GroupKind{}, "", string(cachekey))
	return this.OwnerCache.updateOwnerData(cachekey, key, id, active, nil, valid)
}

////////////////////////////////////////////////////////////////////////////////
// tests

var _ = ginkgov2.Describe("Owner cache", func() {
	config := &Config{
		Ident: ident,
	}
	resources.NewClusterObjectKeySet()
	key1 := resources.NewClusterKeyForObject("test-cluster", resources.NewKey(resources.NewGroupKind("", "test"), "test", "o1"))
	name1 := OwnerName(key1.Name())
	key2 := resources.NewClusterKeyForObject("test-cluster", resources.NewKey(resources.NewGroupKind("", "test"), "test", "o2"))
	name2 := OwnerName(key2.Name())

	var cache *TestOwnerCacheContext

	ginkgov2.BeforeEach(func() {
		cache = &TestOwnerCacheContext{ids: map[OwnerName]owner{}}
		cache.OwnerCache = NewOwnerCache(cache, config)
	})

	ginkgov2.It("initializes the cache correctly", func() {
		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("adds active owner", func() {
		cache.updateOwnerData(name1, "id1", true)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1")))
	})

	ginkgov2.It("adds inactive owner", func() {
		cache.updateOwnerData(name1, "id1", false)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("activate inactive owner", func() {
		cache.updateOwnerData(name1, "id1", false)
		cache.updateOwnerData(name1, "id1", true)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1")))
	})

	ginkgov2.It("deactivate active owner", func() {
		cache.updateOwnerData(name1, "id1", true)
		cache.updateOwnerData(name1, "id1", false)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("readd inactive owner", func() {
		cache.updateOwnerData(name1, ident, false)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("delete readded inactive owner", func() {
		cache.updateOwnerData(name1, ident, false)
		cache.DeleteOwner(key1)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("delete inactive owner", func() {
		cache.updateOwnerData(name1, "id1", false)
		cache.updateOwnerData(name2, "id1", true)
		changed, _ := cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1")))

		changed, _ = cache.DeleteOwner(key2)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("delete readdednil, inactive owner twice", func() {
		cache.updateOwnerData(name1, ident, false)
		cache.DeleteOwner(key1)
		cache.DeleteOwner(key1)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("activate and deactivate two active owner", func() {
		cache.updateOwnerData(name1, ident, false)
		cache.updateOwnerData(name2, ident, true)
		cache.updateOwnerData(name1, ident, true)
		cache.updateOwnerData(name1, ident, false)
		changed, _ := cache.updateOwnerData(name2, ident, false)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
		Expect(changed).To(Equal(utils.NewStringSet()))
	})

	ginkgov2.It("activate and deactivate two active owner", func() {
		changed, _ := cache.updateOwnerData(name1, "id1", false)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.updateOwnerData(name2, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name1, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.updateOwnerData(name2, "id1", false)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1")))
	})

	ginkgov2.It("activate and deactivate two active owner", func() {
		cache.updateOwnerData(name1, "id1", false)
		cache.updateOwnerData(name2, "id1", true)
		cache.updateOwnerData(name1, "id1", true)
		cache.updateOwnerData(name2, "id1", false)
		changed, _ := cache.updateOwnerData(name1, "id1", false)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("activate and delete active owner", func() {
		changed, _ := cache.updateOwnerData(name1, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("activate and delete (one) active two owner2", func() {
		changed, _ := cache.updateOwnerData(name1, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name2, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet()))

		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1")))
	})
	ginkgov2.It("activate and delete (all) active two owner2", func() {
		changed, _ := cache.updateOwnerData(name1, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name2, "id1", true)
		Expect(changed).To(Equal(utils.NewStringSet()))

		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.DeleteOwner(key2)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident)))
	})

	ginkgov2.It("activate and observe expiration date", func() {
		changed, _ := cache.updateOwnerDataExpiration(name1, "id1", true, 1*time.Second)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name2, "id2", true)
		Expect(changed).To(Equal(utils.NewStringSet("id2")))
		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1", "id2")))
		time.Sleep(2 * time.Second)
		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id2")))
		changed, _ = cache.updateOwnerData(name1, "id1", true)
		Expect(cache.GetIds()).To(Equal(utils.NewStringSet(ident, "id1", "id2")))
	})

})
