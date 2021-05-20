/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package integration

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	api "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/provider/mock"
	"github.com/gardener/external-dns-management/pkg/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dns"

	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
)

type FailOption int

const (
	FailGetZones FailOption = iota
	FailDeleteEntry
)

type TestEnv struct {
	Namespace      string
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

	resources.Register(v1alpha1.SchemeBuilder)
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

	awaitCRD := func(max int, crdName string) error {
		var err error
		for i := 0; i < max; i++ {
			err = apiextensions.WaitCRDReady(cluster, crdName)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
			if i%5 == 4 {
				logger.Infof("Still waiting for CRD %s ...", crdName)
			}
		}
		return err
	}

	err = awaitCRD(30, "dnsproviders.dns.gardener.cloud")
	if err != nil {
		return nil, fmt.Errorf("Wait for CRD failed: %s", err)
	}
	err = awaitCRD(30, "dnsentries.dns.gardener.cloud")
	if err != nil {
		return nil, fmt.Errorf("Wait for CRD failed: %s", err)
	}
	return cluster, nil
}

func NewTestEnv(kubeconfig string, namespace string) (*TestEnv, error) {
	logger := logger.NewContext("", "TestEnv")
	cluster, err := waitForCluster(kubeconfig, logger)
	if err != nil {
		return nil, err
	}
	te := &TestEnv{Cluster: cluster, Namespace: namespace, Logger: logger,
		defaultTimeout: 30 * time.Second, resources: cluster.Resources()}
	err = te.CreateNamespace(namespace)
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
	ns := api.Namespace{}
	ns.SetName(namespace)
	_, err := te.resources.CreateOrUpdateObject(&ns)
	return err
}

func (te *TestEnv) SecretName(index int) string {
	return fmt.Sprintf("mock-secret-%d", index)
}

func (te *TestEnv) CreateSecret(index int) (resources.Object, error) {
	name := te.SecretName(index)
	secret := api.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	obj, err := te.resources.CreateOrUpdateObject(&secret)
	return obj, err
}

func BuildProviderConfig(domain, baseDomain string, failOptions ...FailOption) *runtime.RawExtension {
	zones := fmt.Sprintf(`"zones": ["%s","x.%s"]`, domain, baseDomain)
	fail := ""
	for _, opt := range failOptions {
		switch opt {
		case FailGetZones:
			fail += `,"failGetZones": true`
		case FailDeleteEntry:
			fail += `,"failDeleteEntry": true`
		}
	}
	return &runtime.RawExtension{Raw: []byte(fmt.Sprintf("{%s%s}", zones, fail))}
}

func (te *TestEnv) CreateProvider(baseDomain string, providerIndex int, secretName string, failOptions ...FailOption) (resources.Object, string, error) {
	domain := fmt.Sprintf("pr-%d.%s", providerIndex, baseDomain)

	setSpec := func(spec *v1alpha1.DNSProviderSpec) {
		spec.Domains = &v1alpha1.DNSSelection{Include: []string{domain}}
		spec.Type = "mock-inmemory"
		spec.ProviderConfig = BuildProviderConfig(domain, baseDomain, failOptions...)
		spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: te.Namespace}
	}
	obj, err := te.CreateProviderEx(providerIndex, secretName, setSpec)
	return obj, domain, err
}

type ProviderSpecSetter func(p *v1alpha1.DNSProviderSpec)

func (te *TestEnv) CreateProviderEx(providerIndex int, secretName string, setSpec ProviderSpecSetter) (resources.Object, error) {
	name := fmt.Sprintf("mock-provider-%d", providerIndex)
	provider := &v1alpha1.DNSProvider{}
	provider.SetName(name)
	provider.SetNamespace(te.Namespace)
	setSpec(&provider.Spec)
	obj, err := te.resources.CreateObject(provider)
	if errors.IsAlreadyExists(err) {
		te.Infof("Provider %s already existing, updating...", name)
		obj, provider, err = te.GetProvider(name)
		if err == nil {
			setSpec(&provider.Spec)
			err = obj.Update()
		}
	}
	return obj, err
}

func (te *TestEnv) CreateSecretAndProvider(baseDomain string, index int, failOptions ...FailOption) (resources.Object, string, error) {
	secret, err := te.CreateSecret(index)
	if err != nil {
		return nil, "", fmt.Errorf("Creation of secret failed with: %s", err.Error())
	}
	return te.CreateProvider(baseDomain, index, secret.GetName(), failOptions...)
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

type SpecSetter func(e *v1alpha1.DNSEntry)

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

func (te *TestEnv) CreateEntryGeneric(index int, specSetter SpecSetter) (resources.Object, error) {
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
	err := obj.Delete()
	if err != nil {
		return err
	}

	err = te.AwaitEntryDeletion(obj.GetName())
	return err
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
	entries, err := te.resources.GetByExample(&v1alpha1.DNSEntry{})
	if err != nil {
		return nil, err
	}

	objs, err := entries.List(metav1.ListOptions{})
	for _, obj := range objs {
		refs := obj.GetOwnerReferences()
		if refs != nil {
			for _, ref := range refs {
				if ref.Kind == kind && ref.Name == name {
					return obj, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("Entry for %s of kind %s not found", name, kind)
}

func (te *TestEnv) CreateOwner(name, ownerId string) (resources.Object, error) {

	setSpec := func(e *v1alpha1.DNSOwner) {
		e.Spec.OwnerId = ownerId
	}

	owner := &v1alpha1.DNSOwner{}
	owner.SetName(name)
	owner.SetNamespace(te.Namespace)
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
	owner.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&owner)

	if err != nil {
		return nil, err
	}
	return obj, nil
}

func UnwrapOwner(obj resources.Object) *v1alpha1.DNSOwner {
	return obj.Data().(*v1alpha1.DNSOwner)
}

func (te *TestEnv) CreateIngressWithAnnotation(name, domainName string) (resources.Object, error) {
	setter := func(e *networking.Ingress) {
		e.Annotations = map[string]string{"dns.gardener.cloud/dnsnames": domainName}
		e.Spec.Rules = []networking.IngressRule{{Host: domainName}}
	}

	ingress := &networking.Ingress{}
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
	return obj, err
}

func (te *TestEnv) GetIngress(name string) (resources.Object, *networking.Ingress, error) {
	ingress := networking.Ingress{}
	ingress.SetName(name)
	ingress.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&ingress)

	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*networking.Ingress), nil
}

func (te *TestEnv) CreateServiceWithAnnotation(name, domainName, fakeExternalIP string, ttl int) (resources.Object, error) {
	setter := func(e *api.Service) {
		e.Annotations = map[string]string{"dns.gardener.cloud/dnsnames": domainName, "dns.gardener.cloud/ttl": fmt.Sprintf("%d", ttl)}
		e.Spec.Type = corev1.ServiceTypeLoadBalancer
		e.Spec.Ports = []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080), Protocol: corev1.ProtocolTCP}}
	}

	ip := "1.2.3.4"
	service.FakeTargetIP = &ip
	svc := &api.Service{}
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
	return obj, err
}

func (te *TestEnv) GetService(name string) (resources.Object, *api.Service, error) {
	svc := api.Service{}
	svc.SetName(name)
	svc.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&svc)

	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*api.Service), nil
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
	secret := &api.Secret{}
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

func (te *TestEnv) DeleteSecretByName(name string) error {
	secret := &api.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	return te.resources.DeleteObject(secret)
}

func (te *TestEnv) MockInMemoryHasEntry(e resources.Object) error {
	entry := e.Data().(*v1alpha1.DNSEntry)
	dnsSet, err := te.MockInMemoryGetDNSSet(entry.Spec.DNSName)
	if err != nil {
		return err
	}
	if dnsSet == nil {
		return fmt.Errorf("no DNSSet found for %s in mock-inmemory", entry.Spec.DNSName)
	}
	return nil
}

func (te *TestEnv) MockInMemoryGetDNSSet(dnsName string) (*dns.DNSSet, error) {
	for _, zone := range mock.TestMock.GetZones() {
		if zone.Match(dnsName) > 0 {
			state, err := mock.TestMock.CloneZoneState(zone)
			if err != nil {
				return nil, err
			}
			if set := state.GetDNSSets()[dnsName]; set != nil {
				return set, nil
			}
		}
	}
	return nil, nil
}

func (te *TestEnv) MockInMemoryHasNotEntry(e resources.Object) error {
	entry := e.Data().(*v1alpha1.DNSEntry)
	dnsSet, err := te.MockInMemoryGetDNSSet(entry.Spec.DNSName)
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
