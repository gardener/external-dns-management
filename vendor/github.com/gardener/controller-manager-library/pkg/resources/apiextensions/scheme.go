/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package apiextensions

import (
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"
	resources "github.com/gardener/controller-manager-library/pkg/resources/plain"
)

var scheme = runtime.NewScheme()
var decoder runtime.Decoder
var crdGK = resources.NewGroupKind(v1beta1.GroupName, "CustomResourceDefinition")

func init() {
	runtimeutil.Must(v1beta1.AddToScheme(scheme))
	runtimeutil.Must(v1.AddToScheme(scheme))
	runtimeutil.Must(apiextensions.AddToScheme(scheme))
	codecs := serializer.NewCodecFactory(scheme)
	decoder = codecs.UniversalDecoder()

	resources.Register(v1beta1.SchemeBuilder)
	resources.Register(v1.SchemeBuilder)
}

func GetCustomResourceDefinition(spec CRDSpecification) (*CustomResourceDefinition, error) {
	var data []byte
	var err error

	def := &apiextensions.CustomResourceDefinition{}

	switch obj := spec.(type) {
	case *CustomResourceDefinition:
		return obj, nil
	case []byte:
		data = obj
	case string:
		data = []byte(obj)
	case *apiextensions.CustomResourceDefinition:
		return &CustomResourceDefinition{obj}, nil
	case *v1beta1.CustomResourceDefinition, *v1.CustomResourceDefinition:
		err = scheme.Convert(obj, def, nil)
	case runtime.Object:
		err = scheme.Convert(obj, def, nil)
		if err != nil {
			return nil, errors.NewInvalid("invalid CRD spec type: %T", spec)
		}
	default:
		return nil, errors.NewInvalid("invalid CRD spec type: %T", spec)
	}

	if data != nil {
		_, _, err = decoder.Decode([]byte(data), nil, def)
	}
	if err != nil {
		return nil, err
	}
	return &CustomResourceDefinition{def}, nil
}
