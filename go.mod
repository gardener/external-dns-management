module github.com/gardener/external-dns-management

go 1.23.0

toolchain go1.23.2

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.12.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns v1.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns v1.2.0
	github.com/ahmetb/gen-crd-api-reference-docs v0.3.0
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190603021944-12ad9f921c0b
	github.com/aws/aws-sdk-go-v2 v1.32.5
	github.com/aws/aws-sdk-go-v2/config v1.28.5
	github.com/aws/aws-sdk-go-v2/credentials v1.17.46
	github.com/aws/aws-sdk-go-v2/service/route53 v1.46.1
	github.com/aws/smithy-go v1.22.1
	github.com/cloudflare/cloudflare-go v0.11.4
	github.com/gardener/controller-manager-library v0.2.1-0.20241104074533-80cbeddadabc
	github.com/gardener/gardener v1.108.0
	github.com/go-openapi/runtime v0.24.1
	github.com/go-openapi/strfmt v0.21.2
	github.com/gophercloud/gophercloud v0.24.0
	github.com/gophercloud/utils v0.0.0-20220307143606-8e7800759d16
	github.com/infobloxopen/infoblox-go-client/v2 v2.1.0
	github.com/joeig/go-powerdns/v3 v3.10.0
	github.com/miekg/dns v1.1.62
	github.com/netlify/open-api v1.1.0
	github.com/onsi/ginkgo/v2 v2.21.0
	github.com/onsi/gomega v1.35.0
	github.com/prometheus/client_golang v1.20.5
	go.uber.org/atomic v1.10.0
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/oauth2 v0.23.0
	golang.org/x/sync v0.9.0
	golang.org/x/tools v0.27.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/api v0.149.0
	google.golang.org/grpc v1.66.2
	google.golang.org/protobuf v1.35.1
	istio.io/api v1.23.3
	istio.io/client-go v1.23.2
	k8s.io/api v0.31.2
	k8s.io/apiextensions-apiserver v0.31.2
	k8s.io/apimachinery v0.31.2
	k8s.io/client-go v0.31.2
	k8s.io/code-generator v0.31.2
	k8s.io/utils v0.0.0-20241104163129-6fe5fd82f078
	sigs.k8s.io/controller-runtime v0.19.1
	sigs.k8s.io/controller-tools v0.16.5
	sigs.k8s.io/gateway-api v1.0.0
	sigs.k8s.io/kind v0.24.0
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.9.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/alessio/shellescape v1.4.2 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.24 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.24 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.12.1 // indirect
	github.com/evanphx/json-patch/v5 v5.9.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-openapi/analysis v0.21.2 // indirect
	github.com/go-openapi/errors v0.20.4 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/loads v0.21.1 // indirect
	github.com/go-openapi/spec v0.20.4 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-openapi/validate v0.21.0 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/pprof v0.0.0-20241029153458-d1b30febd7db // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/safetext v0.0.0-20220905092116-b49f7bc46da2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/ironcore-dev/vgopath v0.1.5 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.60.1 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.mongodb.org/mongo-driver v1.8.3 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.29.0 // indirect
	golang.org/x/lint v0.0.0-20190930215403-16217165b5de // indirect
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/net v0.31.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	golang.org/x/term v0.26.0 // indirect
	golang.org/x/text v0.20.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/gengo v0.0.0-20230829151522-9cce18d56c01 // indirect
	k8s.io/gengo/v2 v2.0.0-20240228010128-51d4e06bde70 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240808142205-8e686545bdb8 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)

// need open PR https://github.com/infobloxopen/infoblox-go-client/pull/195
replace github.com/infobloxopen/infoblox-go-client/v2 => github.com/MartinWeindel/infoblox-go-client/v2 v2.0.0-20221122085954-b7d80ebb106b
