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
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Owner cache", func() {

	config := &Config{
		Ident: "TEST",
	}
	key1 := resources.NewKey(resources.NewGroupKind("", "test"), "test", "o1")
	name1 := OwnerName(key1.Name())
	key2 := resources.NewKey(resources.NewGroupKind("", "test"), "test", "o2")
	name2 := OwnerName(key2.Name())

	ginkgo.It("initializes the cache correctly", func() {
		cache := NewOwnerCache(config)
		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("adds active  owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", true, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST", "id1")))
	})

	ginkgo.It("adds inactive  owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", false, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("activate inactive  owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", false, nil)
		cache.updateOwnerData(name1, "id1", true, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST", "id1")))
	})

	ginkgo.It("deactivate active  owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", true, nil)
		cache.updateOwnerData(name1, "id1", false, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("readd inactive owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "TEST", false, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("delete readded inactive owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "TEST", false, nil)
		cache.DeleteOwner(key1)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("delete inactive owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", false, nil)
		cache.updateOwnerData(name2, "id1", true, nil)
		changed, _ := cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST", "id1")))

		changed, _ = cache.DeleteOwner(key2)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("delete readded inactive owner twice", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "TEST", false, nil)
		cache.DeleteOwner(key1)
		cache.DeleteOwner(key1)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("activate and deactivate two active owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "TEST", false, nil)
		cache.updateOwnerData(name2, "TEST", true, nil)
		cache.updateOwnerData(name1, "TEST", true, nil)
		cache.updateOwnerData(name1, "TEST", false, nil)
		changed, _ := cache.updateOwnerData(name2, "TEST", false, nil)

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
		Expect(changed).To(Equal(utils.NewStringSet()))
	})

	ginkgo.It("activate and deactivate two active owner", func() {
		cache := NewOwnerCache(config)

		changed, _ := cache.updateOwnerData(name1, "id1", false, nil)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.updateOwnerData(name2, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name1, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.updateOwnerData(name2, "id1", false, nil)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST", "id1")))
	})

	ginkgo.It("activate and deactivate two active owner", func() {
		cache := NewOwnerCache(config)

		cache.updateOwnerData(name1, "id1", false, nil)
		cache.updateOwnerData(name2, "id1", true, nil)
		cache.updateOwnerData(name1, "id1", true, nil)
		cache.updateOwnerData(name2, "id1", false, nil)
		changed, _ := cache.updateOwnerData(name1, "id1", false, nil)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("activate and delete active owner", func() {
		cache := NewOwnerCache(config)

		changed, _ := cache.updateOwnerData(name1, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

	ginkgo.It("activate and delete (one) active two owner2", func() {
		cache := NewOwnerCache(config)

		changed, _ := cache.updateOwnerData(name1, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name2, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet()))

		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST", "id1")))
	})
	ginkgo.It("activate and delete (all) active two owner2", func() {
		cache := NewOwnerCache(config)

		changed, _ := cache.updateOwnerData(name1, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))
		changed, _ = cache.updateOwnerData(name2, "id1", true, nil)
		Expect(changed).To(Equal(utils.NewStringSet()))

		changed, _ = cache.DeleteOwner(key1)
		Expect(changed).To(Equal(utils.NewStringSet()))
		changed, _ = cache.DeleteOwner(key2)
		Expect(changed).To(Equal(utils.NewStringSet("id1")))

		Expect(cache.GetIds()).To(Equal(utils.NewStringSet("TEST")))
	})

})
