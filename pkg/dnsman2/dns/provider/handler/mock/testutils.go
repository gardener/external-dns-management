// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
)

func MarshallMockConfig(input MockConfig) (*runtime.RawExtension, error) {
	bytes, err := json.Marshal(&input)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: bytes}, nil
}
