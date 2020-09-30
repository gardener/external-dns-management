/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
