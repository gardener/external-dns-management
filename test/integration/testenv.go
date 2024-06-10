// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	errs "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"github.com/gardener/controller-manager-library/pkg/utils"
	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	"github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
	"github.com/gardener/external-dns-management/pkg/server/remote"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"
	istioapinetworkingv1 "istio.io/api/networking/v1"
	istionetworkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/ptr"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ProviderTestOption int

const (
	FailGetZones ProviderTestOption = iota
	FailDeleteEntry
	FailSecondZoneWithSameBaseDomain
	AlternativeMockName
	PrivateZones
	Quotas4PerMin
	RemoveAccess
)

type TestEnv struct {
	Namespace      string
	ZonePrefix     string
	Cluster        cluster.Interface
	Logger         logger.LogContext
	defaultTimeout time.Duration
	resources      resources.Resources
}

func doInit() {
	cluster.Configure(
		dnsprovider.PROVIDER_CLUSTER,
		"providers",
		"cluster to look for provider objects",
	).Fallback(dnssource.TARGET_CLUSTER)

	mappings.ForControllerGroup(dnsprovider.CONTROLLER_GROUP_DNS_CONTROLLERS).
		Map(controller.CLUSTER_MAIN, dnssource.TARGET_CLUSTER).MustRegister()

	utils.Must(resources.Register(v1alpha1.SchemeBuilder))
	utils.Must(resources.Register(apiextensionsv1.SchemeBuilder))

	embed.RegisterCreateServerFunc(remote.CreateServer)
}

func runControllerManager(args []string) {
	os.Args = args

	doInit()

	controllermanager.Start("dns-controller-manager", "dns controller manager", "nothing")
}

func waitForCluster(kubeconfig string, logger logger.LogContext) (cluster.Interface, error) {
	req := cluster.DefaultRegistry().GetDefinitions().Get(cluster.DEFAULT)
	ctx := context.Background()
	cfg := &cluster.Config{KubeConfig: kubeconfig}
	cluster, err := cluster.CreateCluster(ctx, logger, req, "", cfg)
	if err != nil {
		return nil, fmt.Errorf("CreateCluster failed: %s", err)
	}
	return cluster, nil
}

func (te *TestEnv) WaitForCRDs() error {
	awaitCRD := func(max int, crdName string) error {
		var err error
		for i := 0; i < max; i++ {
			err = apiextensions.WaitCRDReady(te.Cluster, crdName)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
			if i%5 == 4 {
				te.Logger.Infof("Still waiting for CRD %s ...", crdName)
			}
		}
		return err
	}

	err := awaitCRD(30, "dnsproviders.dns.gardener.cloud")
	if err != nil {
		return fmt.Errorf("Wait for CRD failed: %s", err)
	}
	err = awaitCRD(30, "dnsentries.dns.gardener.cloud")
	if err != nil {
		return fmt.Errorf("Wait for CRD failed: %s", err)
	}
	err = awaitCRD(30, "dnsannotations.dns.gardener.cloud")
	if err != nil {
		return fmt.Errorf("Wait for CRD failed: %s", err)
	}
	return nil
}

func (te *TestEnv) ApplyCRDs(dir string) error {
	resource, err := te.Cluster.Resources().GetByGK(resources.NewGroupKind("apiextensions.k8s.io", "CustomResourceDefinition"))
	if err != nil {
		return err
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
			docs, err := readDocuments(filepath.Join(dir, file.Name()))
			if err != nil {
				return fmt.Errorf("reading files failed: %w", err)
			}

			for i, doc := range docs {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				if err := yaml.Unmarshal(doc, crd); err != nil {
					return fmt.Errorf("unmarshalling doc %s %d failed: %w", file.Name(), i, err)
				}
				if crd.Name == "" {
					continue
				}
				if _, err := resource.CreateOrUpdate(crd); err != nil {
					return fmt.Errorf("crd creation failed: doc %s %d: %w", file.Name(), i, err)
				}
			}
		}
	}
	return nil
}

// readDocuments reads documents from file.
func readDocuments(fp string) ([][]byte, error) {
	b, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}

	docs := [][]byte{}
	reader := yaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(b)))
	for {
		// Read document
		doc, err := reader.Read()
		if err != nil {
			if errs.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func NewTestEnv(kubeconfig string, namespace string) (*TestEnv, error) {
	logger := logger.NewContext("", "TestEnv")
	cluster, err := waitForCluster(kubeconfig, logger)
	if err != nil {
		return nil, err
	}
	te := &TestEnv{
		Cluster:        cluster,
		Namespace:      namespace,
		ZonePrefix:     namespace + ":",
		Logger:         logger,
		defaultTimeout: 30 * time.Second,
		resources:      cluster.Resources(),
	}
	err = te.CreateNamespace(namespace)
	return te, err
}

func NewTestEnvNamespace(first *TestEnv, namespace string) (*TestEnv, error) {
	logger := logger.NewContext("", "TestEnv-"+namespace)
	te := &TestEnv{
		Cluster:        first.Cluster,
		Namespace:      namespace,
		ZonePrefix:     namespace + ":",
		Logger:         logger,
		defaultTimeout: 30 * time.Second,
		resources:      first.Cluster.Resources(),
	}
	err := te.CreateNamespace(namespace)
	return te, err
}

func (te *TestEnv) Infof(msgfmt string, args ...interface{}) {
	te.Logger.Infof(msgfmt, args...)
}

func (te *TestEnv) Warnf(msgfmt string, args ...interface{}) {
	te.Logger.Warnf(msgfmt, args...)
}

func (te *TestEnv) Errorf(msgfmt string, args ...interface{}) {
	te.Logger.Errorf(msgfmt, args...)
}

func (te *TestEnv) CreateNamespace(namespace string) error {
	ns := corev1.Namespace{}
	ns.SetName(namespace)
	_, err := te.resources.CreateOrUpdateObject(&ns)
	return err
}

func (te *TestEnv) SecretName(index int) string {
	return fmt.Sprintf("mock-secret-%d", index)
}

func (te *TestEnv) CreateSecret(index int) (resources.Object, error) {
	name := te.SecretName(index)
	secret := corev1.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	return te.CreateSecretEx(&secret)
}

func (te *TestEnv) CreateSecretEx(secret *corev1.Secret) (resources.Object, error) {
	obj, err := te.resources.CreateOrUpdateObject(secret)
	return obj, err
}

func (te *TestEnv) BuildProviderConfig(domain, domain2 string, failOptions ...ProviderTestOption) *runtime.RawExtension {
	name := te.Namespace
	prefix2 := ""
	for _, opt := range failOptions {
		switch opt {
		case AlternativeMockName:
			name = name + "-alt"
		case PrivateZones:
			prefix2 = "private:"
		}
	}

	input := mock.MockConfig{
		Name: name,
		Zones: []mock.MockZone{
			{ZonePrefix: te.ZonePrefix + prefix2, DNSName: domain},
			{ZonePrefix: te.ZonePrefix + prefix2 + "second:", DNSName: domain2},
		},
	}
	return te.BuildProviderConfigEx(input, failOptions...)
}

func (te *TestEnv) BuildProviderConfigEx(input mock.MockConfig, failOptions ...ProviderTestOption) *runtime.RawExtension {
	for _, opt := range failOptions {
		switch opt {
		case FailGetZones:
			input.FailGetZones = true
		case FailDeleteEntry:
			input.FailDeleteEntry = true
		case FailSecondZoneWithSameBaseDomain:
			input.Zones = append(input.Zones, mock.MockZone{
				ZonePrefix: te.ZonePrefix + ":second",
				DNSName:    input.Zones[0].DNSName,
			})
		}
	}

	bytes, err := json.Marshal(&input)
	if err != nil {
		return nil
	}
	return &runtime.RawExtension{Raw: bytes}
}

func (te *TestEnv) CreateProvider(baseDomain string, providerIndex int, secretName string, options ...ProviderTestOption) (resources.Object, string, string, error) {
	domain := fmt.Sprintf("pr-%d.%s", providerIndex, baseDomain)
	domain2 := fmt.Sprintf("pr-%d-2.%s", providerIndex, baseDomain)

	setSpec := func(provider *v1alpha1.DNSProvider) {
		spec := &provider.Spec
		spec.Domains = &v1alpha1.DNSSelection{Include: []string{domain}}
		spec.Type = "mock-inmemory"
		spec.ProviderConfig = te.BuildProviderConfig(domain, domain2, options...)
		spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: te.Namespace}
		for _, opt := range options {
			switch opt {
			case Quotas4PerMin:
				spec.RateLimit = &v1alpha1.RateLimit{
					RequestsPerDay: 24 * 60 * 4,
					Burst:          1,
				}
			case RemoveAccess:
				resources.SetAnnotation(provider, dnsprovider.AnnotationRemoteAccess, "true")
			}
		}
	}
	obj, err := te.CreateProviderEx(providerIndex, setSpec)
	return obj, domain, domain2, err
}

type ProviderSpecSetter func(p *v1alpha1.DNSProvider)

func (te *TestEnv) CreateProviderEx(providerIndex int, setSpec ProviderSpecSetter) (resources.Object, error) {
	name := fmt.Sprintf("mock-provider-%d", providerIndex)
	provider := &v1alpha1.DNSProvider{}
	provider.SetName(name)
	provider.SetNamespace(te.Namespace)
	setSpec(provider)
	obj, err := te.resources.CreateObject(provider)
	if errors.IsAlreadyExists(err) {
		for i := 0; i < 10; i++ {
			te.Infof("Provider %s already existing, updating...", name)
			obj, provider, err = te.GetProvider(name)
			if err != nil {
				break
			}
			setSpec(provider)
			err = obj.Update()
			if err == nil || !errors.IsConflict(err) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return obj, err
}

func (te *TestEnv) CreateSecretAndProvider(baseDomain string, index int, options ...ProviderTestOption) (resources.Object, string, string, error) {
	secret, err := te.CreateSecret(index)
	if err != nil {
		return nil, "", "", fmt.Errorf("Creation of secret failed with: %s", err.Error())
	}
	return te.CreateProvider(baseDomain, index, secret.GetName(), options...)
}

func (te *TestEnv) DeleteProviderAndSecret(pr resources.Object) error {
	provider := UnwrapProvider(pr)

	err := pr.Delete()
	if err != nil {
		return err
	}

	err = te.AwaitProviderDeletion(pr.GetName())
	if err != nil {
		return err
	}

	err = te.DeleteSecretByName(provider.Spec.SecretRef.Name)
	return err
}

type EntrySpecSetter func(e *v1alpha1.DNSEntry)

func (te *TestEnv) CreateEntry(index int, baseDomain string) (resources.Object, error) {
	target := fmt.Sprintf("1.1.%d.%d", (index/256)%256, index%256)
	ttl := int64(100 + index)

	setSpec := func(e *v1alpha1.DNSEntry) {
		e.Spec.TTL = &ttl
		e.Spec.DNSName = fmt.Sprintf("e%d.%s", index, baseDomain)
		e.Spec.Targets = []string{target}
	}
	return te.CreateEntryGeneric(index, setSpec)
}

func (te *TestEnv) CreateTXTEntry(index int, baseDomain string) (resources.Object, error) {
	txt := fmt.Sprintf("text-%d", index)
	ttl := int64(100 + index)

	setSpec := func(e *v1alpha1.DNSEntry) {
		e.Spec.TTL = &ttl
		e.Spec.DNSName = fmt.Sprintf("e%d.%s", index, baseDomain)
		e.Spec.Text = []string{txt}
	}
	return te.CreateEntryGeneric(index, setSpec)
}

func (te *TestEnv) CreateEntryGeneric(index int, specSetter EntrySpecSetter) (resources.Object, error) {
	name := fmt.Sprintf("mock-entry-%d", index)
	entry := &v1alpha1.DNSEntry{}
	entry.SetName(name)
	entry.SetNamespace(te.Namespace)
	specSetter(entry)
	obj, err := te.resources.CreateObject(entry)
	if errors.IsAlreadyExists(err) {
		te.Infof("Entry %s already existing, updating...", name)
		obj, err = te.GetEntry(name)
		if err == nil {
			specSetter(UnwrapEntry(obj))
			err = obj.Update()
		}
	}
	return obj, err
}

func (te *TestEnv) UpdateEntryOwner(obj resources.Object, ownerID *string) (resources.Object, error) {
	obj, err := te.GetEntry(obj.GetName())
	if err != nil {
		return nil, err
	}
	e := UnwrapEntry(obj)
	e.Spec.OwnerId = ownerID
	err = obj.Update()
	return obj, err
}

func (te *TestEnv) UpdateEntryDomain(obj resources.Object, domain string) (resources.Object, error) {
	obj, err := te.GetEntry(obj.GetName())
	if err != nil {
		return nil, err
	}
	e := UnwrapEntry(obj)
	e.Spec.DNSName = domain
	err = obj.Update()
	return obj, err
}

func (te *TestEnv) UpdateEntryTargets(obj resources.Object, targets ...string) (resources.Object, error) {
	obj, err := te.GetEntry(obj.GetName())
	if err != nil {
		return nil, err
	}
	e := UnwrapEntry(obj)
	if len(targets) == 0 {
		e.Spec.Targets = nil
	} else {
		e.Spec.Targets = targets
	}
	err = obj.Update()
	return obj, err
}

func (te *TestEnv) DeleteEntryAndWait(obj resources.Object) error {
	return te.DeleteEntriesAndWait(obj)
}

func (te *TestEnv) DeleteEntriesAndWait(objs ...resources.Object) error {
	for _, obj := range objs {
		err := obj.Delete()
		if err != nil {
			return err
		}
	}

	for _, obj := range objs {
		err := te.AwaitEntryDeletion(obj.GetName())
		if err != nil {
			return err
		}
	}
	return nil
}

func (te *TestEnv) GetEntry(name string) (resources.Object, error) {
	entry := v1alpha1.DNSEntry{}
	entry.SetName(name)
	entry.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&entry)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func UnwrapEntry(obj resources.Object) *v1alpha1.DNSEntry {
	return obj.Data().(*v1alpha1.DNSEntry)
}

func (te *TestEnv) FindEntryByOwner(kind, name string) (resources.Object, error) {
	objs, err := te.FindEntriesByOwner(kind, name)
	if err != nil {
		return nil, err
	}
	switch len(objs) {
	case 1:
		return objs[0], nil
	case 0:
		return nil, fmt.Errorf("Entry for %s of kind %s not found", name, kind)
	default:
		return nil, fmt.Errorf("multiple entries for %s of kind %s", name, kind)
	}
}

func (te *TestEnv) FindEntriesByOwner(kind, name string) ([]resources.Object, error) {
	entries, err := te.resources.GetByExample(&v1alpha1.DNSEntry{})
	if err != nil {
		return nil, err
	}

	var foundObjs []resources.Object
	objs, err := entries.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		refs := obj.GetOwnerReferences()
		for _, ref := range refs {
			if ref.Kind == kind && ref.Name == name {
				foundObjs = append(foundObjs, obj)
			}
		}
	}
	return foundObjs, nil
}

func (te *TestEnv) CreateOwner(name, ownerId string) (resources.Object, error) {
	setSpec := func(e *v1alpha1.DNSOwner) {
		e.Spec.OwnerId = ownerId
	}

	return te.CreateOwnerGeneric(name, setSpec)
}

type OwnerSpecSetter func(e *v1alpha1.DNSOwner)

func (te *TestEnv) CreateOwnerGeneric(name string, setSpec OwnerSpecSetter) (resources.Object, error) {
	owner := &v1alpha1.DNSOwner{}
	owner.SetName(name)
	setSpec(owner)
	obj, err := te.resources.CreateObject(owner)
	if errors.IsAlreadyExists(err) {
		te.Infof("DNSOwner %s already existing, updating...", name)
		obj, err = te.GetOwner(name)
		if err == nil {
			setSpec(UnwrapOwner(obj))
			err = obj.Update()
		}
	}
	return obj, err
}

func (te *TestEnv) GetOwner(name string) (resources.Object, error) {
	owner := v1alpha1.DNSOwner{}
	owner.SetName(name)
	obj, err := te.resources.GetObject(&owner)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (te *TestEnv) DeleteOwner(obj resources.Object) error {
	return obj.Delete()
}

func UnwrapOwner(obj resources.Object) *v1alpha1.DNSOwner {
	return obj.Data().(*v1alpha1.DNSOwner)
}

func (te *TestEnv) CreateIngressWithAnnotation(name, domainName, fakeExternalIP string, ttl int, routingPolicy *string,
	additionalAnnotations map[string]string,
) (resources.Object, error) {
	setter := func(e *networkingv1.Ingress) {
		e.Annotations = map[string]string{dnssource.DNS_ANNOTATION: "*", dnssource.TTL_ANNOTATION: fmt.Sprintf("%d", ttl)}
		if routingPolicy != nil {
			e.Annotations[dnssource.ROUTING_POLICY_ANNOTATION] = *routingPolicy
		}
		for k, v := range additionalAnnotations {
			e.Annotations[k] = v
		}
		e.Spec.Rules = []networkingv1.IngressRule{
			{
				Host:             domainName,
				IngressRuleValue: networkingv1.IngressRuleValue{},
			},
		}
	}

	ingress := &networkingv1.Ingress{}
	ingress.SetName(name)
	ingress.SetNamespace(te.Namespace)
	setter(ingress)
	obj, err := te.resources.CreateObject(ingress)
	if errors.IsAlreadyExists(err) {
		te.Infof("Ingress %s already existing, updating...", name)
		obj, ingress, err = te.GetIngress(name)
		if err == nil {
			setter(ingress)
			err = obj.Update()
		}
	}
	if err != nil {
		return obj, err
	}

	if fakeExternalIP != "" {
		err := te.PatchIngressLoadBalancer(obj, fakeExternalIP)
		if err != nil {
			return obj, err
		}
	}

	return obj, err
}

func (te *TestEnv) PatchIngressLoadBalancer(ingressObj resources.Object, fakeExternalIP string) error {
	ingress, ok := ingressObj.Data().(*networkingv1.Ingress)
	if !ok {
		return fmt.Errorf("not an ingress object")
	}
	res, err := te.resources.Get(ingress)
	if err != nil {
		return err
	}
	_, _, err = res.ModifyStatus(ingress, func(data resources.ObjectData) (bool, error) {
		o := data.(*networkingv1.Ingress)
		if fakeExternalIP != "" {
			o.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
				{IP: fakeExternalIP},
			}
		} else {
			o.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{}
		}
		return true, nil
	})
	return err
}

func (te *TestEnv) GetIngress(name string) (resources.Object, *networkingv1.Ingress, error) {
	ingress := networkingv1.Ingress{}
	ingress.SetName(name)
	ingress.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&ingress)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*networkingv1.Ingress), nil
}

func (te *TestEnv) CreateServiceWithAnnotation(name, domainName string, status *corev1.LoadBalancerIngress, ttl int,
	routingPolicy *string, additionalAnnotations map[string]string,
) (resources.Object, error) {
	setter := func(e *corev1.Service) {
		e.Annotations = map[string]string{dnssource.DNS_ANNOTATION: domainName, dnssource.TTL_ANNOTATION: fmt.Sprintf("%d", ttl)}
		if routingPolicy != nil {
			e.Annotations[dnssource.ROUTING_POLICY_ANNOTATION] = *routingPolicy
		}
		for k, v := range additionalAnnotations {
			e.Annotations[k] = v
		}
		e.Spec.Type = corev1.ServiceTypeLoadBalancer
		e.Spec.Ports = []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}}
	}

	svc := &corev1.Service{}
	svc.SetName(name)
	svc.SetNamespace(te.Namespace)
	setter(svc)
	obj, err := te.resources.CreateObject(svc)
	if errors.IsAlreadyExists(err) {
		te.Infof("Service %s already existing, updating...", name)
		obj, svc, err = te.GetService(name)
		if err == nil {
			setter(svc)
			err = obj.Update()
		}
	}
	if err != nil {
		return obj, err
	}

	if status != nil {
		res, err := te.resources.Get(svc)
		if err != nil {
			return obj, err
		}
		_, _, err = res.ModifyStatus(svc, func(data resources.ObjectData) (bool, error) {
			o := data.(*corev1.Service)
			o.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{*status}
			return true, nil
		})
		if err != nil {
			return obj, err
		}
	}

	return obj, err
}

func (te *TestEnv) GetService(name string) (resources.Object, *corev1.Service, error) {
	svc := corev1.Service{}
	svc.SetName(name)
	svc.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&svc)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*corev1.Service), nil
}

func (te *TestEnv) HasEntryState(name string, states ...string) (bool, error) {
	obj, err := te.GetEntry(name)
	if err != nil {
		return false, err
	}
	entry := UnwrapEntry(obj)
	found := false
	for _, state := range states {
		found = found || entry.Status.State == state
	}
	return found, nil
}

func (te *TestEnv) GetProvider(name string) (resources.Object, *v1alpha1.DNSProvider, error) {
	provider := &v1alpha1.DNSProvider{}
	provider.SetName(name)
	provider.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(provider)
	if err != nil {
		return nil, nil, err
	}
	return obj, UnwrapProvider(obj), nil
}

func UnwrapProvider(obj resources.Object) *v1alpha1.DNSProvider {
	return obj.Data().(*v1alpha1.DNSProvider)
}

func (te *TestEnv) UpdateProviderSpec(obj resources.Object, f func(spec *v1alpha1.DNSProviderSpec) error) (resources.Object, error) {
	obj, pr, err := te.GetProvider(obj.GetName())
	if err != nil {
		return nil, err
	}
	err = f(&pr.Spec)
	if err != nil {
		return nil, err
	}
	err = obj.Update()
	return obj, err
}

func (te *TestEnv) GetSecret(name string) (resources.Object, error) {
	secret := &corev1.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(secret)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (te *TestEnv) HasProviderState(name string, states ...string) (bool, error) {
	_, provider, err := te.GetProvider(name)
	if err != nil {
		return false, err
	}
	found := false
	for _, state := range states {
		found = found || provider.Status.State == state
	}
	return found, nil
}

func (te *TestEnv) CreateDNSAnnotationForService(name string, spec v1alpha1.DNSAnnotationSpec) (resources.Object, error) {
	annot := &v1alpha1.DNSAnnotation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: te.Namespace,
			Name:      name,
		},
		Spec: spec,
	}

	obj, err := te.resources.CreateObject(annot)
	if errors.IsAlreadyExists(err) {
		te.Infof("Service %s already existing, updating...", name)
		obj, annot, err = te.GetDNSAnnotation(name)
		if err == nil {
			annot.Spec = spec
			err = obj.Update()
		}
	}
	return obj, err
}

func (te *TestEnv) CreateServiceAndIstioGatewayWithAnnotation(name, domainName string, status *corev1.LoadBalancerIngress, ttl int,
	routingPolicy *string, additionalAnnotations map[string]string,
) (resources.Object, resources.Object, error) {
	selector := map[string]string{"istio": "ingressgateway"}
	svcSetter := func(e *corev1.Service) {
		e.Labels = selector
		e.Spec.Type = corev1.ServiceTypeLoadBalancer
		e.Spec.Ports = []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}}
	}

	svc := &corev1.Service{}
	svc.SetName(name)
	svc.SetNamespace(te.Namespace)
	svcSetter(svc)
	svcObj, err := te.resources.CreateObject(svc)
	if errors.IsAlreadyExists(err) {
		te.Infof("Service %s already existing, updating...", name)
		svcObj, svc, err = te.GetService(name)
		if err == nil {
			svcSetter(svc)
			err = svcObj.Update()
		}
	}
	if err != nil {
		return svcObj, nil, err
	}

	gwObj, err := te.CreateIstioGatewayWithAnnotation(name, domainName, selector, ttl, routingPolicy, additionalAnnotations)
	if err != nil {
		return svcObj, gwObj, err
	}

	if status != nil {
		res, err := te.resources.Get(svcObj)
		if err != nil {
			return svcObj, gwObj, err
		}
		_, _, err = res.ModifyStatus(svcObj.Data(), func(data resources.ObjectData) (bool, error) {
			o := data.(*corev1.Service)
			o.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{*status}
			return true, nil
		})
		if err != nil {
			return svcObj, gwObj, err
		}
	}

	return svcObj, gwObj, err
}

func (te *TestEnv) CreateIstioGatewayWithAnnotation(name, domainName string, selector map[string]string, ttl int,
	routingPolicy *string, additionalAnnotations map[string]string,
) (resources.Object, error) {
	setter := func(gw *istionetworkingv1.Gateway) {
		gw.Annotations = map[string]string{dnssource.DNS_ANNOTATION: "*", dnssource.TTL_ANNOTATION: fmt.Sprintf("%d", ttl)}
		if routingPolicy != nil {
			gw.Annotations[dnssource.ROUTING_POLICY_ANNOTATION] = *routingPolicy
		}
		for k, v := range additionalAnnotations {
			gw.Annotations[k] = v
		}
		gw.Spec.Servers = []*istioapinetworkingv1.Server{
			{
				Port: &istioapinetworkingv1.Port{
					Name:     "http",
					Number:   80,
					Protocol: "HTTP",
				},
				Hosts: []string{domainName},
			},
		}
		gw.Spec.Selector = selector
	}

	gw := &istionetworkingv1.Gateway{}
	gw.SetName(name)
	gw.SetNamespace(te.Namespace)
	setter(gw)
	obj, err := te.resources.CreateObject(gw)
	if errors.IsAlreadyExists(err) {
		te.Infof("IstioGateway %s already existing, updating...", name)
		obj, gw, err = te.GetIstioGateway(name)
		if err == nil {
			setter(gw)
			err = obj.Update()
		}
	}
	return obj, err
}

func (te *TestEnv) CreateIngressAndIstioGatewayWithAnnotation(
	name string,
	domainName string,
	status *networkingv1.IngressLoadBalancerIngress,
	ttl int,
	routingPolicy *string,
) (resources.Object, resources.Object, error) {
	selector := map[string]string{}
	ingressSetter := func(e *networkingv1.Ingress) {
		e.Spec.Rules = []networkingv1.IngressRule{
			{
				Host:             domainName,
				IngressRuleValue: networkingv1.IngressRuleValue{},
			},
		}
	}

	ingress := &networkingv1.Ingress{}
	ingress.SetName(name)
	ingress.SetNamespace(te.Namespace)
	ingressSetter(ingress)
	svcObj, err := te.resources.CreateObject(ingress)
	if errors.IsAlreadyExists(err) {
		te.Infof("Ingress %s already existing, updating...", name)
		svcObj, ingress, err = te.GetIngress(name)
		if err == nil {
			ingressSetter(ingress)
			err = svcObj.Update()
		}
	}
	if err != nil {
		return svcObj, nil, err
	}

	additionalAnnotations := map[string]string{istio.IngressTargetSourceAnnotation: fmt.Sprintf("%s/%s", ingress.Namespace, ingress.Name)}
	gwObj, err := te.CreateIstioGatewayWithAnnotation(name, domainName, selector, ttl, routingPolicy, additionalAnnotations)
	if err != nil {
		return svcObj, gwObj, err
	}
	if err != nil {
		return svcObj, gwObj, err
	}

	if status != nil {
		res, err := te.resources.Get(svcObj)
		if err != nil {
			return svcObj, gwObj, err
		}
		_, _, err = res.ModifyStatus(svcObj.Data(), func(data resources.ObjectData) (bool, error) {
			o := data.(*networkingv1.Ingress)
			o.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{*status}
			return true, nil
		})
		if err != nil {
			return svcObj, gwObj, err
		}
	}

	return svcObj, gwObj, err
}

func (te *TestEnv) GetIstioGateway(name string) (resources.Object, *istionetworkingv1.Gateway, error) {
	gw := istionetworkingv1.Gateway{}
	gw.SetName(name)
	gw.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&gw)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*istionetworkingv1.Gateway), nil
}

func (te *TestEnv) CreateGatewayAPIGatewayWithAnnotation(name, domainName string, address *gatewayapisv1.GatewayStatusAddress, ttl int,
	routingPolicy *string, additionalAnnotations map[string]string,
) (resources.Object, error) {
	setter := func(gw *gatewayapisv1.Gateway) {
		gw.Annotations = map[string]string{dnssource.DNS_ANNOTATION: "*", dnssource.TTL_ANNOTATION: fmt.Sprintf("%d", ttl)}
		if routingPolicy != nil {
			gw.Annotations[dnssource.ROUTING_POLICY_ANNOTATION] = *routingPolicy
		}
		for k, v := range additionalAnnotations {
			gw.Annotations[k] = v
		}
		gw.Spec.GatewayClassName = "test"
		gw.Spec.Listeners = []gatewayapisv1.Listener{
			{
				Name:     "listener1",
				Protocol: gatewayapisv1.HTTPProtocolType,
				Port:     80,
			},
		}
		if domainName != "" {
			gw.Spec.Listeners[0].Hostname = ptr.To(gatewayapisv1.Hostname(domainName))
		}
	}

	gw := &gatewayapisv1.Gateway{}
	gw.SetName(name)
	gw.SetNamespace(te.Namespace)
	setter(gw)
	obj, err := te.resources.CreateObject(gw)
	if errors.IsAlreadyExists(err) {
		te.Infof("Gateway %s already existing, updating...", name)
		obj, gw, err = te.GetGatewayAPIGateway(name)
		if err == nil {
			setter(gw)
			err = obj.Update()
		}
	}
	if err != nil {
		return obj, err
	}

	if address != nil {
		res, err := te.resources.Get(obj)
		if err != nil {
			return obj, err
		}
		_, _, err = res.ModifyStatus(obj.Data(), func(data resources.ObjectData) (bool, error) {
			o := data.(*gatewayapisv1.Gateway)
			o.Status.Addresses = []gatewayapisv1.GatewayStatusAddress{*address}
			return true, nil
		})
		if err != nil {
			return obj, err
		}
	}

	return obj, err
}

func (te *TestEnv) GetGatewayAPIGateway(name string) (resources.Object, *gatewayapisv1.Gateway, error) {
	gw := gatewayapisv1.Gateway{}
	gw.SetName(name)
	gw.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&gw)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*gatewayapisv1.Gateway), nil
}

func (te *TestEnv) CreateGatewayAPIHTTPRoute(name, hostname string, gateway resources.ObjectName) (resources.Object, error) {
	setter := func(gw *gatewayapisv1.HTTPRoute) {
		gw.Spec.Hostnames = []gatewayapisv1.Hostname{gatewayapisv1.Hostname(hostname)}
		gw.Spec.ParentRefs = []gatewayapisv1.ParentReference{
			{
				Namespace: ptr.To(gatewayapisv1.Namespace(gateway.Namespace())),
				Name:      gatewayapisv1.ObjectName(gateway.Name()),
			},
		}
	}

	route := &gatewayapisv1.HTTPRoute{}
	route.SetName(name)
	route.SetNamespace(te.Namespace)
	setter(route)
	obj, err := te.resources.CreateObject(route)
	if errors.IsAlreadyExists(err) {
		te.Infof("HTTPRoute %s already existing, updating...", name)
		obj, route, err = te.GetGatewayAPIHTTPRoute(name)
		if err == nil {
			setter(route)
			err = obj.Update()
		}
	}

	return obj, err
}

func (te *TestEnv) GetGatewayAPIHTTPRoute(name string) (resources.Object, *gatewayapisv1.HTTPRoute, error) {
	gw := gatewayapisv1.HTTPRoute{}
	gw.SetName(name)
	gw.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&gw)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*gatewayapisv1.HTTPRoute), nil
}

func (te *TestEnv) GetDNSAnnotation(name string) (resources.Object, *v1alpha1.DNSAnnotation, error) {
	annot := &v1alpha1.DNSAnnotation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: te.Namespace,
			Name:      name,
		},
	}

	obj, err := te.resources.GetObject(&annot)
	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*v1alpha1.DNSAnnotation), nil
}

func (te *TestEnv) AwaitEntryReady(name string) error {
	return te.AwaitEntryState(name, "Ready")
}

func (te *TestEnv) AwaitEntryStale(name string) error {
	return te.AwaitEntryState(name, "Stale")
}

func (te *TestEnv) AwaitEntryInvalid(name string) error {
	return te.AwaitEntryState(name, "Invalid")
}

func (te *TestEnv) AwaitEntryError(name string) error {
	return te.AwaitEntryState(name, "Error")
}

func (te *TestEnv) AwaitEntryState(name string, states ...string) error {
	msg := fmt.Sprintf("Entry %s state=%v", name, states)
	return te.Await(msg, func() (bool, error) {
		return te.HasEntryState(name, states...)
	})
}

func (te *TestEnv) AwaitProviderReady(name string) error {
	return te.AwaitProviderState(name, "Ready")
}

func (te *TestEnv) AwaitProviderState(name string, states ...string) error {
	msg := fmt.Sprintf("Provider %s state=%v", name, states)
	return te.Await(msg, func() (bool, error) {
		return te.HasProviderState(name, states...)
	})
}

type CheckFunc func() (bool, error)

func (te *TestEnv) Await(msg string, check CheckFunc) error {
	return te.AwaitWithTimeout(msg, check, te.defaultTimeout)
}

func (te *TestEnv) AwaitWithTimeout(msg string, check CheckFunc, timeout time.Duration) error {
	var err error
	var ok bool

	limit := time.Now().Add(timeout)
	for time.Now().Before(limit) {
		ok, err = check()
		if ok {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("Timeout during check %s with error %s", msg, err.Error())
	}
	return fmt.Errorf("Timeout during check  %s", msg)
}

func (te *TestEnv) AwaitProviderDeletion(name string) error {
	msg := fmt.Sprintf("Provider %s still existing", name)
	return te.Await(msg, func() (bool, error) {
		_, _, err := te.GetProvider(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) AwaitEntryDeletion(name string) error {
	msg := fmt.Sprintf("Entry %s still existing", name)
	return te.Await(msg, func() (bool, error) {
		_, err := te.GetEntry(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) AwaitSecretDeletion(name string) error {
	msg := fmt.Sprintf("Secret %s still existing", name)
	return te.Await(msg, func() (bool, error) {
		_, err := te.GetSecret(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) AwaitServiceDeletion(name string) error {
	msg := fmt.Sprintf("Service %s still existing", name)
	return te.Await(msg, func() (bool, error) {
		_, _, err := te.GetService(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) AwaitIngressDeletion(name string) error {
	msg := fmt.Sprintf("Ingress %s still existing", name)
	return te.Await(msg, func() (bool, error) {
		_, _, err := te.GetIngress(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) AwaitObjectByOwner(kind, name string) (resources.Object, error) {
	var entryObj resources.Object
	err := te.Await("Generated entry for service not found", func() (bool, error) {
		var err error
		entryObj, err = te.FindEntryByOwner(kind, name)
		if entryObj != nil {
			return true, nil
		}
		return false, err
	})
	return entryObj, err
}

func (te *TestEnv) AwaitObjectsByOwner(kind, name string, count int) ([]resources.Object, error) {
	var objs []resources.Object
	err := te.Await(fmt.Sprintf("Generated entries for %s %s not found", kind, name), func() (bool, error) {
		var err error
		objs, err = te.FindEntriesByOwner(kind, name)
		return len(objs) == count, err
	})
	return objs, err
}

func (te *TestEnv) DeleteSecretByName(name string) error {
	secret := &corev1.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	return te.resources.DeleteObject(secret)
}

func (te *TestEnv) MockInMemoryHasEntry(e resources.Object) error {
	return te.MockInMemoryHasEntryEx(te.Namespace, te.ZonePrefix, e)
}

func (te *TestEnv) MockInMemoryHasEntryEx(name, zonePrefix string, e resources.Object) error {
	entry := e.Data().(*v1alpha1.DNSEntry)
	dnsSet, err := te.MockInMemoryGetDNSSetEx(name, zonePrefix, entry.Spec.DNSName)
	if err != nil {
		return err
	}
	if dnsSet == nil {
		return fmt.Errorf("no DNSSet found for %s in mock-inmemory", entry.Spec.DNSName)
	}
	return nil
}

func (te *TestEnv) MockInMemoryGetDNSSet(dnsName string) (*dns.DNSSet, error) {
	return te.MockInMemoryGetDNSSetEx(te.Namespace, te.ZonePrefix, dnsName)
}

func (te *TestEnv) MockInMemoryGetDNSSetEx(name, zonePrefix, dnsName string) (*dns.DNSSet, error) {
	testMock := mock.TestMock[name]
	if testMock == nil {
		return nil, nil
	}
	for _, zone := range testMock.GetZones() {
		if strings.HasPrefix(zone.Id().ID, zonePrefix) && zone.Match(dnsName) > 0 {
			state, err := testMock.CloneZoneState(zone)
			if err != nil {
				return nil, err
			}
			if set := state.GetDNSSets()[dns.DNSSetName{DNSName: dnsName}]; set != nil {
				return set, nil
			}
		}
	}
	return nil, nil
}

func (te *TestEnv) MockInMemoryHasNotEntry(e resources.Object) error {
	return te.MockInMemoryHasNotEntryEx(te.Namespace, te.ZonePrefix, e)
}

func (te *TestEnv) MockInMemoryHasNotEntryEx(name, zonePrefix string, e resources.Object) error {
	entry := e.Data().(*v1alpha1.DNSEntry)
	dnsSet, err := te.MockInMemoryGetDNSSetEx(name, zonePrefix, entry.Spec.DNSName)
	if err != nil {
		return err
	}
	if dnsSet != nil {
		return fmt.Errorf("DNSSet found for %s in mock-inmemory", entry.Spec.DNSName)
	}
	return nil
}

func same(lst1 []string, lst2 []string) bool {
	if len(lst1) == len(lst2) {
		found := 0
		for _, f1 := range lst1 {
			for _, f2 := range lst2 {
				if f1 == f2 {
					found++
					break
				}
			}
		}
		return found == len(lst1)
	}
	return false
}

func (te *TestEnv) AwaitFinalizers(obj resources.Object, expectedFinalizers ...string) error {
	finalizers := obj.GetFinalizers()
	if same(finalizers, expectedFinalizers) {
		return nil
	}

	msg := fmt.Sprintf("Expected finalizers %v for %s", expectedFinalizers, obj.GetName())
	return te.Await(msg, func() (bool, error) {
		obj, err := te.resources.GetObject(obj.Key())
		if err == nil {
			finalizers := obj.GetFinalizers()
			te.Infof("finalizers: %v %v", finalizers, expectedFinalizers)
			if same(finalizers, expectedFinalizers) {
				return true, nil
			}
			err = fmt.Errorf("actual finalizers=%v", finalizers)
		}
		return false, err
	})
}
