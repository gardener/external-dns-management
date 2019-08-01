module github.com/gardener/external-dns-management

go 1.12

require (
	cloud.google.com/go v0.36.0 // indirect
	contrib.go.opencensus.io/exporter/ocagent v0.2.0 // indirect
	github.com/Azure/azure-sdk-for-go v24.1.0+incompatible
	github.com/Azure/go-autorest v11.4.0+incompatible
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190603021944-12ad9f921c0b
	github.com/aws/aws-sdk-go v1.19.41
	github.com/census-instrumentation/opencensus-proto v0.1.0 // indirect
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/dimchansky/utfbom v1.1.0 // indirect
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/gardener/controller-manager-library v0.0.0-20190827084958-1a1296828114
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/gofuzz v0.0.0-20170612174753-24818f796faf // indirect
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/gophercloud/gophercloud v0.2.0
	github.com/hashicorp/golang-lru v0.5.0 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/miekg/dns v1.1.14
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	github.com/pkg/errors v0.8.1 // indirect
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model v0.0.0-20190129233127-fd36f4220a90 // indirect
	github.com/prometheus/common v0.2.0 // indirect
	github.com/prometheus/procfs v0.0.0-20190328153300-af7bedc223fb // indirect
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/spf13/cobra v0.0.3 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	golang.org/x/lint v0.0.0-20181026193005-c67002cb31c3
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/oauth2 v0.0.0-20190130055435-99b60b757ec1
	google.golang.org/api v0.1.0
	google.golang.org/grpc v1.18.0 // indirect
	k8s.io/api v0.0.0-20190708174958-539a33f6e817
	k8s.io/apiextensions-apiserver v0.0.0-20190208173442-01b4f3c039db
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v0.0.0-20190708175433-62e1c231c5dc
	k8s.io/code-generator v0.0.0-20190311093542-50b561225d70
	k8s.io/gengo v0.0.0-20190327210449-e17681d19d3a // indirect
	k8s.io/klog v0.3.3 // indirect
	k8s.io/kube-openapi v0.0.0-20190722073852-5e22f3d471e6 // indirect
	k8s.io/utils v0.0.0-20190712204705-3dccf664f023 // indirect
	sigs.k8s.io/yaml v1.1.0 // indirect
)

replace (
	k8s.io/api => k8s.io/api v0.0.0-20190708174958-539a33f6e817 // kubernetes-1.14.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190208173442-01b4f3c039db
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d // kubernetes-1.14.4
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190708175433-62e1c231c5dc // kubernetes-1.14.4
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190311093542-50b561225d70 // kubernetes-1.14.4
)
