/*
Copyright (c) YEAR SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package crds

import (
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

var registry = apiextensions.NewRegistry()

func init() {
	var data string
	data = `

---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.2.4
  creationTimestamp: null
  name: dnsannotations.dns.gardener.cloud
spec:
  group: dns.gardener.cloud
  names:
    kind: DNSAnnotation
    listKind: DNSAnnotationList
    plural: dnsannotations
    shortNames:
    - dnsa
    singular: dnsannotation
  scope: Namespaced
  versions:
  - additionalPrinterColumns:
    - jsonPath: .spec.resourceRef.apiVersion
      name: RefGroup
      type: string
    - jsonPath: .spec.resourceRef.kind
      name: RefKind
      type: string
    - jsonPath: .spec.resourceRef.name
      name: RefName
      type: string
    - jsonPath: .spec.resourceRef.namespace
      name: RefNamespace
      type: string
    - jsonPath: .status.active
      name: Active
      type: boolean
    - jsonPath: .metadata.creationTimestamp
      name: Age
      type: date
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              annotations:
                additionalProperties:
                  type: string
                type: object
              resourceRef:
                properties:
                  apiVersion:
                    description: API Version of the annotated object
                    type: string
                  kind:
                    description: 'Kind of the annotated object More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
                    type: string
                  name:
                    description: Name of the annotated object
                    type: string
                  namespace:
                    description: Namspace of the annotated object Defaulted by the
                      namespace of the containing resource.
                    type: string
                required:
                - apiVersion
                - kind
                type: object
            required:
            - annotations
            - resourceRef
            type: object
          status:
            properties:
              active:
                description: Indicates that annotation is observed by a DNS sorce
                  controller
                type: boolean
              message:
                description: In case of a configuration problem this field describes
                  the reason
                type: string
            type: object
        required:
        - spec
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
  `
	utils.Must(registry.RegisterCRD(data))
	data = `

---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.2.4
  creationTimestamp: null
  name: dnsentries.dns.gardener.cloud
spec:
  group: dns.gardener.cloud
  names:
    kind: DNSEntry
    listKind: DNSEntryList
    plural: dnsentries
    shortNames:
    - dnse
    singular: dnsentry
  scope: Namespaced
  versions:
  - additionalPrinterColumns:
    - description: FQDN of DNS Entry
      jsonPath: .spec.dnsName
      name: DNS
      type: string
    - jsonPath: .spec.ownerId
      name: OWNERID
      type: string
    - jsonPath: .status.providerType
      name: TYPE
      type: string
    - jsonPath: .status.provider
      name: PROVIDER
      type: string
    - jsonPath: ..status.state
      name: STATUS
      type: string
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              cnameLookupInterval:
                format: int64
                type: integer
              dnsName:
                type: string
              ownerId:
                type: string
              reference:
                properties:
                  name:
                    type: string
                  namespace:
                    type: string
                required:
                - name
                type: object
              targets:
                items:
                  type: string
                type: array
              text:
                items:
                  type: string
                type: array
              ttl:
                format: int64
                type: integer
            required:
            - dnsName
            type: object
          status:
            properties:
              message:
                type: string
              observedGeneration:
                format: int64
                type: integer
              provider:
                type: string
              providerType:
                type: string
              state:
                type: string
              targets:
                items:
                  type: string
                type: array
              ttl:
                format: int64
                type: integer
              zone:
                type: string
            type: object
        required:
        - spec
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
  `
	utils.Must(registry.RegisterCRD(data))
	data = `

---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.2.4
  creationTimestamp: null
  name: dnsowners.dns.gardener.cloud
spec:
  group: dns.gardener.cloud
  names:
    kind: DNSOwner
    listKind: DNSOwnerList
    plural: dnsowners
    shortNames:
    - dnso
    singular: dnsowner
  scope: Cluster
  versions:
  - additionalPrinterColumns:
    - jsonPath: .spec.ownerId
      name: OwnerId
      type: string
    - jsonPath: .spec.active
      name: Active
      type: boolean
    - jsonPath: .status.amount
      name: Usages
      type: string
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              active:
                type: boolean
              ownerId:
                type: string
            required:
            - ownerId
            type: object
          status:
            properties:
              entries:
                properties:
                  amount:
                    type: integer
                  types:
                    additionalProperties:
                      type: integer
                    type: object
                type: object
            type: object
        required:
        - spec
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
  `
	utils.Must(registry.RegisterCRD(data))
	data = `

---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.2.4
  creationTimestamp: null
  name: dnsproviders.dns.gardener.cloud
spec:
  group: dns.gardener.cloud
  names:
    kind: DNSProvider
    listKind: DNSProviderList
    plural: dnsproviders
    shortNames:
    - dnspr
    singular: dnsprovider
  scope: Namespaced
  versions:
  - additionalPrinterColumns:
    - jsonPath: .spec.type
      name: TYPE
      type: string
    - jsonPath: ..status.state
      name: STATUS
      type: string
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              domains:
                properties:
                  exclude:
                    items:
                      type: string
                    type: array
                  include:
                    items:
                      type: string
                    type: array
                type: object
              providerConfig:
                type: object
              secretRef:
                description: SecretReference represents a Secret Reference. It has
                  enough information to retrieve secret in any namespace
                properties:
                  name:
                    description: Name is unique within a namespace to reference a
                      secret resource.
                    type: string
                  namespace:
                    description: Namespace defines the space within which the secret
                      name must be unique.
                    type: string
                type: object
              type:
                type: string
              zones:
                properties:
                  exclude:
                    items:
                      type: string
                    type: array
                  include:
                    items:
                      type: string
                    type: array
                type: object
            type: object
          status:
            properties:
              domains:
                properties:
                  excluded:
                    items:
                      type: string
                    type: array
                  included:
                    items:
                      type: string
                    type: array
                type: object
              message:
                type: string
              observedGeneration:
                format: int64
                type: integer
              state:
                type: string
              zones:
                properties:
                  excluded:
                    items:
                      type: string
                    type: array
                  included:
                    items:
                      type: string
                    type: array
                type: object
            type: object
        required:
        - spec
        type: object
    served: true
    storage: true
    subresources:
      status: {}
status:
  acceptedNames:
    kind: ""
    plural: ""
  conditions: []
  storedVersions: []
  `
	utils.Must(registry.RegisterCRD(data))
}

func AddToRegistry(r apiextensions.Registry) {
	registry.AddToRegistry(r)
}
