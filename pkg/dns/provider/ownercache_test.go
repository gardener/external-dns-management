// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const ident = "TEST"

////////////////////////////////////////////////////////////////////////////////
// test driver

type owner struct {
	id     string
	active bool
}
type TestOwnerCacheContext struct {
	ids map[OwnerName]owner
	*OwnerCache
}

func (this *TestOwnerCacheContext) GetContext() context.Context {
	return context.Background()
}

func (this *TestOwnerCacheContext) Infof(_ string, _ ...interface{}) {
}

func (this *TestOwnerCacheContext) EnqueueKey(key resources.ClusterObjectKey) error {
	cachekey := OwnerName(key.Name())
	old, ok := this.ids[cachekey]
	if ok {
		this.OwnerCache.updateOwnerData(cachekey, old.id, old.active, nil)
	}
	return nil
}

func (this *TestOwnerCacheContext) updateOwnerData(cachekey OwnerName, id string, active bool) (changeset utils.StringSet, activeset utils.StringSet) {
	return this._updateOwnerData(cachekey, id, active)
}

func (this *TestOwnerCacheContext) _updateOwnerData(cachekey OwnerName, id string, active bool) (changeset utils.StringSet, activeset utils.StringSet) {
	this.ids[cachekey] = owner{
		id:     id,
		active: active,
	}
	return this.OwnerCache.updateOwnerData(cachekey, id, active, nil)
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
})
