module github.com/gardener/external-dns-management

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v39.0.0+incompatible
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.3.0 // indirect
	github.com/ahmetb/gen-crd-api-reference-docs v0.1.5
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190603021944-12ad9f921c0b
	github.com/aws/aws-sdk-go v1.19.41
	github.com/cloudflare/cloudflare-go v0.11.4
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/gardener/controller-manager-library v0.1.1-0.20200204110458-c263b9bb97ad
	github.com/go-openapi/swag v0.19.4 // indirect
	github.com/gophercloud/gophercloud v0.2.0
	github.com/gophercloud/utils v0.0.0-20190527093828-25f1b77b8c03
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e // indirect
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/miekg/dns v1.1.14
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/prometheus/client_golang v0.9.3
	github.com/spf13/cobra v0.0.6 // indirect
	golang.org/x/lint v0.0.0-20190313153728-d0100b6bd8b3
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20200317113312-5766fd39f98d // indirect
	golang.org/x/tools v0.0.0-20191127201027-ecd32218bd7f // indirect
	google.golang.org/api v0.4.0
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/api v0.16.4
	k8s.io/apiextensions-apiserver v0.16.4
	k8s.io/apimachinery v0.17.0
	//k8s.io/apimachinery v0.17.0
	k8s.io/client-go v0.16.4
	k8s.io/code-generator v0.16.4
	k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	sigs.k8s.io/controller-tools v0.2.4
	sigs.k8s.io/kind v0.7.0
)

//gopkg.in/fsnotify.v1 v1.4.7 => github.com/fsnotify/fsnotify v1.4.7
replace (
	github.com/gardener/controller-manager-library => ../controller-manager-library
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.4
)
