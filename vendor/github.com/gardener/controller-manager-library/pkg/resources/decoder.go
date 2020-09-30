/*
SPDX-FileCopyrightText: 2018 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

// taken from controller runtime project

package resources

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/json"
)

////////////////////////////////////////////////////////////////////////////////

// Decoder knows how to decode the contents of an admission
// request into a concrete object.
type Decoder struct {
	scheme  *runtime.Scheme
	codecs  serializer.CodecFactory
	decoder runtime.Decoder
}

// NewDecoder creates a Decoder given the runtime.Scheme
func NewDecoder(scheme *runtime.Scheme) *Decoder {
	codecs := serializer.NewCodecFactory(scheme)
	return &Decoder{scheme: scheme, codecs: codecs, decoder: UniversalDecoder(scheme, codecs.UniversalDeserializer())}
}

func (d *Decoder) CodecFactory() serializer.CodecFactory {
	return d.codecs
}

// Decode decodes the inlined object.
func (d *Decoder) Decode(content []byte) (runtime.Object, *schema.GroupVersionKind, error) {
	return d.decoder.Decode(content, nil, nil)
}

// DecodeInto decodes on object given as byte stream into a runtimeObject or
// similar Object
func (d *Decoder) DecodeInto(data []byte, into interface{}) error {
	switch target := into.(type) {
	case *unstructured.Unstructured:
		// unmarshal into unstructured's underlying object to avoid calling the decoder
		if err := json.Unmarshal(data, &target.Object); err != nil {
			return err
		}
		return nil
	case versionedObjects:
		_, _, err := d.decoder.Decode(data, nil, target)
		return err
	case runtime.Object:
		_, _, err := d.decoder.Decode(data, nil, target)
		return err
	default:
		if err := json.Unmarshal(data, &target); err != nil {
			return err
		}
		return nil
	}
}

// DecodeFromUnstructued decodes an unstruvtured object into a structured one
func (d *Decoder) DecodeFromUnstructued(data *unstructured.Unstructured, into runtime.Object) error {
	return d.DecodeFromMap(data.Object, into)
}

// DecodeFromMap decodes from a map into a runtime Object.
// data is a JSON compatible map with string, float, int, bool, []interface{}, or
// map[string]interface{}
// children.
func (d *Decoder) DecodeFromMap(data map[string]interface{}, into runtime.Object) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return d.DecodeInto(bytes, into)
}

// DecodeRaw decodes a RawExtension object into the passed-in runtime.Object.
func (d *Decoder) DecodeRaw(rawObj runtime.RawExtension, into interface{}) error {
	if rawObj.Size() > 0 {
		return d.DecodeInto(rawObj.Raw, into)
	}
	return nil
}
