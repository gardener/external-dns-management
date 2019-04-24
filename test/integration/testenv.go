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

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	api "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"

	dnsprovider "github.com/gardener/external-dns-management/pkg/dns/provider"
	dnssource "github.com/gardener/external-dns-management/pkg/dns/source"
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

}

func runControllerManager(args []string) {
	os.Args = args

	doInit()

	controllermanager.Start("dns-controller-manager", "dns controller manager", "nothing")
}

func waitForCluster(kubeconfig string, logger logger.LogContext) (cluster.Interface, error) {
	req := cluster.DefaultRegistry().GetDefinitions().Get(cluster.DEFAULT)
	ctx := context.Background()
	cluster, err := cluster.CreateCluster(ctx, logger, req, "", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("CreateCluster failed: %s", err)
	}

	for i := 0; i < 30; i++ {
		err = apiextensions.WaitCRDReady(cluster, "dnsproviders.dns.gardener.cloud")
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
		if i%5 == 4 {
			logger.Infof("Still waiting for CRDs...")
		}
	}
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
		defaultTimeout: 20 * time.Second, resources: cluster.Resources()}
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

func (te *TestEnv) CreateSecret(index int) (string, error) {
	name := fmt.Sprintf("mock-secret-%d", index)
	secret := api.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	_, err := te.resources.CreateOrUpdateObject(&secret)
	return name, err
}

func (te *TestEnv) CreateProvider(baseDomain string, providerIndex int, secretName string) (resources.Object, string, error) {
	name := fmt.Sprintf("mock-provider-%d", providerIndex)
	domain := fmt.Sprintf("pr-%d.%s", providerIndex, baseDomain)

	setSpec := func(p *v1alpha1.DNSProvider) {
		p.Spec.Domains = &v1alpha1.DNSDomainSpec{Include: []string{domain}}
		p.Spec.Type = "mock-inmemory"
		p.Spec.ProviderConfig = &runtime.RawExtension{Raw: []byte(fmt.Sprintf("{\"zones\": [\"%s\"]}", domain))}
		p.Spec.SecretRef = &corev1.SecretReference{Name: secretName, Namespace: te.Namespace}
	}

	provider := &v1alpha1.DNSProvider{}
	provider.SetName(name)
	provider.SetNamespace(te.Namespace)
	setSpec(provider)
	obj, err := te.resources.CreateObject(provider)
	if errors.IsAlreadyExists(err) {
		te.Infof("Provider %s already existing, updating...", name)
		obj, provider, err = te.GetProvider(name)
		if err == nil {
			setSpec(provider)
			err = obj.Update()
		}
	}
	return obj, domain, err
}

func (te *TestEnv) CreateSecretAndProvider(baseDomain string, index int) (resources.Object, string, error) {
	secretName, err := te.CreateSecret(index)
	if err != nil {
		return nil, "", fmt.Errorf("Creation of secret failed with: %s", err.Error())
	}
	return te.CreateProvider(baseDomain, index, secretName)
}

func (te *TestEnv) DeleteProviderAndSecret(pr resources.Object) error {
	provider := pr.Data().(*v1alpha1.DNSProvider)

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

func (te *TestEnv) CreateEntry(index int, baseDomain string) (resources.Object, error) {
	name := fmt.Sprintf("mock-entry-%d", index)
	target := fmt.Sprintf("1.1.%d.%d", (index/256)%256, index%256)
	ttl := int64(100 + index)

	setSpec := func(e *v1alpha1.DNSEntry) {
		e.Spec.TTL = &ttl
		e.Spec.DNSName = fmt.Sprintf("e%d.%s", index, baseDomain)
		e.Spec.Targets = []string{target}
	}

	entry := &v1alpha1.DNSEntry{}
	entry.SetName(name)
	entry.SetNamespace(te.Namespace)
	setSpec(entry)
	obj, err := te.resources.CreateObject(entry)
	if errors.IsAlreadyExists(err) {
		te.Infof("Entry %s already existing, updating...", name)
		obj, entry, err = te.GetEntry(name)
		if err == nil {
			setSpec(entry)
			err = obj.Update()
		}
	}
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

func (te *TestEnv) GetEntry(name string) (resources.Object, *v1alpha1.DNSEntry, error) {
	entry := v1alpha1.DNSEntry{}
	entry.SetName(name)
	entry.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&entry)

	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*v1alpha1.DNSEntry), nil
}

func (te *TestEnv) HasEntryState(name string, states ...string) (bool, error) {
	_, entry, err := te.GetEntry(name)
	if err != nil {
		return false, err
	}
	found := false
	for _, state := range states {
		found = found || entry.Status.State == state
	}
	return found, nil
}

func (te *TestEnv) GetProvider(name string) (resources.Object, *v1alpha1.DNSProvider, error) {
	provider := v1alpha1.DNSProvider{}
	provider.SetName(name)
	provider.SetNamespace(te.Namespace)
	obj, err := te.resources.GetObject(&provider)

	if err != nil {
		return nil, nil, err
	}
	return obj, obj.Data().(*v1alpha1.DNSProvider), nil
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
		fmt.Errorf("Timeout during check %s with error %s", msg, err.Error())
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
		_, _, err := te.GetEntry(name)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func (te *TestEnv) DeleteSecretByName(name string) error {
	secret := api.Secret{}
	secret.SetName(name)
	secret.SetNamespace(te.Namespace)
	return te.resources.DeleteObject(&secret)
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