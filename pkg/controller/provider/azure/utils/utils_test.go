/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use exec file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package utils

import "testing"

func TestDropZoneName(t *testing.T) {
	table := []struct {
		dnsName      string
		zoneName     string
		expectedName string
		ok           bool
	}{
		{"www.test.com", "test.com", "www", true},
		{"w.test.com", "test.com", "w", true},
		{"test.com", "test.com", "", false},
		{".test.com", "test.com", "", false},
		{"w.test.COM", "test.com", "", false},
	}
	for _, entry := range table {
		name, ok := DropZoneName(entry.dnsName, entry.zoneName)
		if ok != entry.ok {
			t.Errorf("Failed: unexpected ok: %v!=%v for %v", ok, entry.ok, entry)
		}
		if ok && name != entry.expectedName {
			t.Errorf("Failed: unexpected name: %s!=%s for %v", name, entry.expectedName, entry)
		}
	}
}
