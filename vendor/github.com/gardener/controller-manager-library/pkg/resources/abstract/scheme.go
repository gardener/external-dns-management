/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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
 *
 */

package abstract

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

type acceptGroupVersioner struct {
}

func (acceptGroupVersioner) KindForGroupVersionKinds(kinds []schema.GroupVersionKind) (schema.GroupVersionKind, bool) {
	for _, kind := range kinds {
		return kind, true
	}
	return schema.GroupVersionKind{}, false
}

type Decoder struct {
	scheme         *runtime.Scheme
	codecfactory   serializer.CodecFactory
	parametercodec runtime.ParameterCodec
	decoder        runtime.Decoder
}

func NewDecoder(scheme *runtime.Scheme) *Decoder {
	decoder := &Decoder{
		scheme:         scheme,
		codecfactory:   serializer.NewCodecFactory(scheme),
		parametercodec: runtime.NewParameterCodec(scheme),
	}
	// decoder.decoder=decoder.codecfactory.CodecForVersions(nil, decoder.codecfactory.UniversalDeserializer(), nil, acceptGroupVersioner{})
	decoder.decoder = decoder.codecfactory.UniversalDeserializer()
	return decoder
}

func (this *Decoder) Decode(bytes []byte) (runtime.Object, *schema.GroupVersionKind, error) {
	return this.decoder.Decode(bytes, nil, nil)
}

func (this *Decoder) DecodeInto(bytes []byte, into runtime.Object) error {
	if unstructuredInto, isUnstructured := into.(*unstructured.Unstructured); isUnstructured {
		// unmarshal into unstructured's underlying object to avoid calling the decoder
		if err := json.Unmarshal(bytes, &unstructuredInto.Object); err != nil {
			return err
		}
		return nil
	}

	return runtime.DecodeInto(this.codecfactory.UniversalDecoder(), bytes, into)
}
