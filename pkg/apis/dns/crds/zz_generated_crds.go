/*
SPDX-FileCopyrightText: YEAR SAP SE or an SAP affiliate company and Gardener contributors

SPDX-License-Identifier: Apache-2.0
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
    controller-gen.kubebuilder.io/version: v0.2.9
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
            description: 'APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
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
                    description: Namspace of the annotated object Defaulted by the namespace of the containing resource.
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
                description: Indicates that annotation is observed by a DNS sorce controller
                type: boolean
              message:
                description: In case of a configuration problem this field describes the reason
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
    controller-gen.kubebuilder.io/version: v0.2.9
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
    - jsonPath: .status.state
      name: STATUS
      type: string
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              cnameLookupInterval:
                description: lookup interval for CNAMEs that must be resolved to IP addresses
                format: int64
                type: integer
              dnsName:
                description: full qualified domain name
                type: string
              ownerId:
                description: owner id used to tag entries in external DNS system
                type: string
              reference:
                description: reference to base entry used to inherit attributes from
                properties:
                  name:
                    description: name of the referenced DNSEntry object
                    type: string
                  namespace:
                    description: namespace of the referenced DNSEntry object
                    type: string
                required:
                - name
                type: object
              targets:
                description: target records (CNAME or A records), either text or targets must be specified
                items:
                  type: string
                type: array
              text:
                description: text records, either text or targets must be specified
                items:
                  type: string
                type: array
              ttl:
                description: time to live for records in external DNS system
                format: int64
                type: integer
            required:
            - dnsName
            type: object
          status:
            properties:
              lastUpdateTime:
                description: lastUpdateTime contains the timestamp of the last status update
                format: date-time
                type: string
              message:
                description: message describing the reason for the state
                type: string
              observedGeneration:
                format: int64
                type: integer
              provider:
                description: assigned provider
                type: string
              providerType:
                description: provider type used for the entry
                type: string
              state:
                description: entry state
                type: string
              targets:
                description: effective targets generated for the entry
                items:
                  type: string
                type: array
              ttl:
                description: time to live used for the entry
                format: int64
                type: integer
              zone:
                description: zone used for the entry
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
    controller-gen.kubebuilder.io/version: v0.2.9
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
    - jsonPath: .status.entries.amount
      name: Usages
      type: integer
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              active:
                description: state of the ownerid for the DNS controller observing entry using this owner id (default:true)
                type: boolean
              ownerId:
                description: owner id used to tag entries in external DNS system
                type: string
            required:
            - ownerId
            type: object
          status:
            properties:
              entries:
                description: Entry statistic for this owner id
                properties:
                  amount:
                    description: number of entries using this owner id
                    type: integer
                  types:
                    additionalProperties:
                      type: integer
                    description: number of entries per provider type
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
    controller-gen.kubebuilder.io/version: v0.2.9
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
    - jsonPath: .status.state
      name: STATUS
      type: string
    name: v1alpha1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            properties:
              defaultTTL:
                description: default TTL used for DNS entries if not specified explicitly
                format: int64
                type: integer
              domains:
                description: desired selection of usable domains (by default all zones and domains in those zones will be served)
                properties:
                  exclude:
                    description: values that should be ignored (domains or zones)
                    items:
                      type: string
                    type: array
                  include:
                    description: values that should be observed (domains or zones)
                    items:
                      type: string
                    type: array
                type: object
              providerConfig:
                description: optional additional provider specific configuration values
                type: object
                x-kubernetes-preserve-unknown-fields: true
              secretRef:
                description: access credential for the external DNS system of the given type
                properties:
                  name:
                    description: Name is unique within a namespace to reference a secret resource.
                    type: string
                  namespace:
                    description: Namespace defines the space within which the secret name must be unique.
                    type: string
                type: object
              type:
                description: type of the provider (selecting the responsible type of DNS controller)
                type: string
              zones:
                description: desired selection of usable domains the domain selection is used for served zones, only (by default all zones will be served)
                properties:
                  exclude:
                    description: values that should be ignored (domains or zones)
                    items:
                      type: string
                    type: array
                  include:
                    description: values that should be observed (domains or zones)
                    items:
                      type: string
                    type: array
                type: object
            type: object
          status:
            properties:
              defaultTTL:
                description: actually used default TTL for DNS entries
                format: int64
                type: integer
              domains:
                description: actually served domain selection
                properties:
                  excluded:
                    description: Excluded values (domains or zones)
                    items:
                      type: string
                    type: array
                  included:
                    description: included values (domains or zones)
                    items:
                      type: string
                    type: array
                type: object
              lastUpdateTime:
                description: lastUpdateTime contains the timestamp of the last status update
                format: date-time
                type: string
              message:
                description: message describing the reason for the actual state of the provider
                type: string
              observedGeneration:
                format: int64
                type: integer
              state:
                description: state of the provider
                type: string
              zones:
                description: actually served zones
                properties:
                  excluded:
                    description: Excluded values (domains or zones)
                    items:
                      type: string
                    type: array
                  included:
                    description: included values (domains or zones)
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
