module github.com/gardener/external-dns-management

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v39.0.0+incompatible
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/to v0.3.0 // indirect
	github.com/ahmetb/gen-crd-api-reference-docs v0.2.0
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190603021944-12ad9f921c0b
	github.com/aws/aws-sdk-go v1.19.41
	github.com/cloudflare/cloudflare-go v0.11.4
	github.com/emicklei/go-restful v2.9.6+incompatible // indirect
	github.com/gardener/controller-manager-library v0.2.1-0.20200814085853-45032cce52c3
	github.com/gophercloud/gophercloud v0.2.0
	github.com/gophercloud/utils v0.0.0-20190527093828-25f1b77b8c03
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/infobloxopen/infoblox-go-client v1.1.0
	github.com/mattn/go-isatty v0.0.12 // indirect
	github.com/miekg/dns v1.1.14
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.0.0
	go.uber.org/atomic v1.4.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.4.0
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/code-generator v0.18.6
	k8s.io/kube-openapi v0.0.0-20200410145947-bcb3869e6f29
	sigs.k8s.io/controller-tools v0.2.9
	sigs.k8s.io/kind v0.7.0
)

replace github.com/infobloxopen/infoblox-go-client => github.com/MartinWeindel/infoblox-go-client v1.1.1-0.20200616154106-b2951ec7a129

// fix Vulnerability CVE-2020-14040
replace golang.org/x/text => golang.org/x/text v0.3.3
