// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdc_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	globalnetworkingv1 "github.com/googlecloudplatform/google-distributed-cloud-apis/pkg/apis/public/global/networking/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	gdcclient "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/gdch"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/gdcloud"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/helm"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/kubernetes"
)

var (
	commitHash          string
	zone                string
	gdchProject         string
	vuc                 string
	org                 string
	labURL              string
	caDataPath          string
	serviceaccountPath  string
	imageWithTag        string
	chartPackagePath    string
	managedDNSZone      string
	priorityClassName   = "gardener-system-900"
	dnsDeploymentLabels = map[string]string{"app": "dns-controller-manager"}
)

const (
	// Ready status condition string used by GDC networking CRDs
	statusReady = "Ready"
)

type config struct {
	namespace          string
	vucClient          client.WithWatch
	globalClient       client.WithWatch
	saSecretName       string
	gdchProject        string
	managedDNSZone     string
	dnsDeploymentLabel map[string]string
}

func init() {
	flag.StringVar(&commitHash, "commit_hash", "", "Commit hash that triggered the presubmit targets")
	flag.StringVar(&zone, "zone", "", "The GDCH zone to be used for running the tests")
	flag.StringVar(&gdchProject, "project", "", "The GDCH project where all resources will be created")
	flag.StringVar(&vuc, "vuc", "", "Vanilla User Cluster on GDCH where tests will be staged")
	flag.StringVar(&org, "org", "", "Organization name for GDCH instance")
	flag.StringVar(&labURL, "lab_url", "", "Gpc demo labs url matching the right environment")
	flag.StringVar(&caDataPath, "cafile", "", "Certificate Authority file path")
	flag.StringVar(&serviceaccountPath, "service_account", "", "Service account key file path")
	flag.StringVar(&imageWithTag, "image_tag", "", "Full image path and tag for dns-controller-manager")
	flag.StringVar(&chartPackagePath, "chart_package", "", "File path to packaged gardener-extension-shoot-dns-service chart")
	flag.StringVar(&managedDNSZone, "managed_dns_zone", "", "Public ManagedDNSZone for the project")
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func setup(ctx context.Context, t *testing.T) *config {
	t.Helper()

	consoleURL := fmt.Sprintf("https://console.%s.%s.%s", org, zone, labURL)
	gdcloudClient, err := gdcloud.NewTestingClient(caDataPath, serviceaccountPath, consoleURL)
	if err != nil {
		t.Fatalf("Failed to initialize gdcloud client: %v", err)
	}
	t.Cleanup(func() {
		if err := gdcloudClient.Cleanup(); err != nil {
			t.Errorf("Failed to cleanup gdcloud config: %v", err)
		}
	})

	vucScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(vucScheme); err != nil {
		t.Fatalf("Failed to add default k8s scheme: %v", err)
	}
	if err := dnsv1alpha1.AddToScheme(vucScheme); err != nil {
		t.Fatalf("Failed to add dnsv1alpha1 scheme: %v", err)
	}

	vucClient, err := gdch.GetUserClusterClient(gdcloudClient, zone, gdchProject, vuc, gdch.WithScheme(vucScheme))
	if err != nil {
		t.Fatalf("Cannot get Kubernetes user cluster client: %v", err)
	}

	globalScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(globalScheme); err != nil {
		t.Fatalf("Failed to add default scheme to globalScheme: %v", err)
	}
	if err := globalnetworkingv1.AddToScheme(globalScheme); err != nil {
		t.Fatalf("Failed to add globalnetworkingv1 to globalScheme: %v", err)
	}

	globalClient, err := gdch.GetGlobalClient(org, zone, labURL, caDataPath, serviceaccountPath, gdch.WithScheme(globalScheme))
	if err != nil {
		t.Fatalf("Cannot get GDCH global cluster client: %v", err)
	}

	t.Logf("Ensuring PriorityClass %s exists", priorityClassName)
	priorityClass := &schedulingv1.PriorityClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: priorityClassName,
		},
		Value:         900000000,
		GlobalDefault: false,
		Description:   "Priority class for Gardener system components",
	}
	if err := vucClient.WatchClient.Create(ctx, priorityClass); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create PriorityClass %s: %v", priorityClassName, err)
	}

	namespace := fmt.Sprintf("ext-dns-mgmt-%s", commitHash)
	t.Logf("Creating namespace %s for tests", namespace)
	if err := kubernetes.CreateNamespace(ctx, vucClient.WatchClient, namespace); err != nil {
		t.Fatalf("Failed to create namespace %s: %v", namespace, err)
	}
	t.Cleanup(func() {
		kubernetes.CleanupResources(t, vucClient.WatchClient, namespace)
	})

	saSecretName, err := createCloudProviderSecret(ctx, vucClient.WatchClient, namespace)
	if err != nil {
		t.Fatalf("Failed to create cloud provider secret: %v", err)
	}

	parts := strings.Split(imageWithTag, ":")
	if len(parts) != 2 {
		t.Fatalf("Invalid image_tag format. Expected 'repo:tag', got '%s'", imageWithTag)
	}
	imageRepo := parts[0]
	tag := parts[1]

	releaseName := fmt.Sprintf("dns-service-%s", commitHash)

	values := map[string]interface{}{
		"replicaCount": 0,
		"dnsControllerManager": map[string]interface{}{
			"deploy":     true,
			"createCRDs": true,
			"image": map[string]interface{}{
				"repository": imageRepo,
				"tag":        tag,
			},
			"configuration": map[string]interface{}{
				"controllers":           "gdch-dns,dnssources",
				"serverPortHttp":        8080,
				"ttl":                   120,
				"rescheduleDelay":       "120s",
				"lockStatusCheckPeriod": "5s",
				"poolResyncPeriod":      "30s",
			},
		},
	}

	t.Logf("Installing chart %s as release %s in namespace %s", chartPackagePath, releaseName, namespace)
	err = helm.InstallOrUpgrade(helm.InstallOptions{
		ChartPath:      chartPackagePath,
		KubeconfigPath: vucClient.GetKubeConfigPath(),
		ReleaseName:    releaseName,
		Namespace:      namespace,
		Values:         values,
		Timeout:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Failed to install helm chart: %v", err)
	}
	t.Cleanup(func() {
		t.Logf("Uninstalling helm release %s in namespace %s", releaseName, namespace)
		if err := helm.Uninstall(helm.UninstallOptions{
			KubeconfigPath: vucClient.GetKubeConfigPath(),
			ReleaseName:    releaseName,
			Namespace:      namespace,
		}); err != nil {
			t.Errorf("Failed to uninstall chart: %v", err)
		}
	})

	deploymentName := "dns-controller-manager"
	t.Logf("Waiting for deployment %s to be ready", deploymentName)
	err = kubernetes.WaitForDeploymentReady(ctx, vucClient.WatchClient, namespace, deploymentName, 3*time.Minute)
	if err != nil {
		t.Fatalf("Deployment %s did not become ready: %v", deploymentName, err)
	}

	podList := &corev1.PodList{}
	err = vucClient.WatchClient.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels(dnsDeploymentLabels))
	if err != nil || len(podList.Items) == 0 {
		t.Fatalf("Failed to list pods for deployment %s or no pods found: %v", deploymentName, err)
	}

	for _, pod := range podList.Items {
		t.Logf("Waiting for pod %s to be ready", pod.Name)
		if err := kubernetes.WaitForPodReady(ctx, vucClient.WatchClient, namespace, pod.Name, 3*time.Minute); err != nil {
			t.Fatalf("Pod %s did not become ready: %v", pod.Name, err)
		}
	}

	t.Log("dns-controller-manager is successfully deployed and ready!")

	return &config{
		namespace:          namespace,
		vucClient:          vucClient.WatchClient,
		globalClient:       globalClient,
		saSecretName:       saSecretName,
		gdchProject:        gdchProject,
		managedDNSZone:     managedDNSZone,
		dnsDeploymentLabel: dnsDeploymentLabels,
	}
}

func createCloudProviderSecret(ctx context.Context, k8sClient client.WithWatch, namespace string) (string, error) {
	saBytes, err := os.ReadFile(serviceaccountPath)
	if err != nil {
		return "", fmt.Errorf("failed to read service account file: %w", err)
	}

	caBytes, err := os.ReadFile(caDataPath)
	if err != nil {
		return "", fmt.Errorf("failed to read ca file: %w", err)
	}

	gdchCfg := gdcclient.OrgClusterConfig{
		OrgClusterURL: fmt.Sprintf("https://global-api.%s.%s.%s", org, zone, labURL),
		CAData:        base64.StdEncoding.EncodeToString(caBytes),
	}

	gdchCfgBytes, err := json.Marshal(gdchCfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OrgClusterConfig: %w", err)
	}

	secretName := fmt.Sprintf("sa-%s", commitHash)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"serviceaccount.json": saBytes,
			"gdch-config":         gdchCfgBytes,
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil {
		return "", fmt.Errorf("failed to create secret: %w", err)
	}
	return secretName, nil
}

func TestExternalDNSManagement(t *testing.T) {
	if commitHash == "" {
		t.Skip("Skipping GDC integration test: -commit_hash flag not provided")
	}

	ctx := context.Background()
	testConfig := setup(ctx, t)

	providerName := "gdc-test-provider"
	dnsProvider := &dnsv1alpha1.DNSProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      providerName,
			Namespace: testConfig.namespace,
		},
		Spec: dnsv1alpha1.DNSProviderSpec{
			Type: "gdch-dns",
			SecretRef: &corev1.SecretReference{
				Name:      testConfig.saSecretName,
				Namespace: testConfig.namespace,
			},
			Domains: &dnsv1alpha1.DNSSelection{
				Include: []string{testConfig.managedDNSZone},
			},
		},
	}

	t.Logf("Creating DNSProvider %s", providerName)
	if err := testConfig.vucClient.Create(ctx, dnsProvider); err != nil {
		t.Fatalf("Failed to create DNSProvider: %v", err)
	}
	t.Cleanup(func() {
		t.Logf("Deleting DNSProvider %s", providerName)
		if err := testConfig.vucClient.Delete(ctx, dnsProvider); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("Failed to delete DNSProvider: %v", err)
		}
	})

	t.Logf("Waiting for DNSProvider %s to become Ready", providerName)
	providerList := &dnsv1alpha1.DNSProviderList{}
	err := kubernetes.WaitForCondition[*dnsv1alpha1.DNSProvider](
		ctx, 2*time.Minute,
		func() (watch.Interface, error) {
			return testConfig.vucClient.Watch(ctx, providerList, client.InNamespace(testConfig.namespace), client.MatchingFields{"metadata.name": providerName})
		},
		func(p *dnsv1alpha1.DNSProvider) bool {
			return p.Status.State == statusReady
		},
	)
	if err != nil {
		var p dnsv1alpha1.DNSProvider
		_ = testConfig.vucClient.Get(ctx, client.ObjectKey{Namespace: testConfig.namespace, Name: providerName}, &p)
		t.Fatalf("DNSProvider %s did not become Ready: %v (Status: %+v)", providerName, err, p.Status)
	}
	t.Logf("DNSProvider %s is Ready!", providerName)

	tests := []struct {
		name            string
		entryName       string
		dnsName         string
		recordType      string
		initialTargets  []string
		initialText     []string
		expectedInitial []string
		updatedTargets  []string
		updatedText     []string
		expectedUpdated []string
	}{
		{
			name:            "Verify GDC DNSEntry for A-record",
			entryName:       "test-a-record",
			dnsName:         fmt.Sprintf("a-record-test.%s", testConfig.managedDNSZone),
			recordType:      "A",
			initialTargets:  []string{"1.2.3.4"},
			expectedInitial: []string{"1.2.3.4"},
			updatedTargets:  []string{"5.6.7.8"},
			expectedUpdated: []string{"5.6.7.8"},
		},
		{
			name:            "Verify GDC DNSEntry for TXT-record",
			entryName:       "test-txt-record",
			dnsName:         fmt.Sprintf("txt-record-test.%s", testConfig.managedDNSZone),
			recordType:      "TXT",
			initialText:     []string{"initial-verification-token"},
			expectedInitial: []string{"\"initial-verification-token\""},
			updatedText:     []string{"updated-verification-token"},
			expectedUpdated: []string{"\"updated-verification-token\""},
		},
	}

	t.Run("DNSRecordsGroup", func(t *testing.T) {
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				verifyDNSEntry(
					ctx, t, testConfig,
					tc.entryName, tc.dnsName, tc.recordType,
					tc.initialTargets, tc.initialText, tc.expectedInitial,
					tc.updatedTargets, tc.updatedText, tc.expectedUpdated,
				)
			})
		}
	})

	verifyPodHealth(ctx, t, testConfig)
}

func verifyDNSEntry(
	ctx context.Context,
	t *testing.T,
	testConfig *config,
	entryName, dnsName, recordType string,
	initialTargets, initialText, expectedInitial []string,
	updatedTargets, updatedText, expectedUpdated []string,
) {
	t.Helper()

	dnsEntry := &dnsv1alpha1.DNSEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      entryName,
			Namespace: testConfig.namespace,
			Annotations: map[string]string{
				"dns.gardener.cloud/class": "gardendns",
			},
		},
		Spec: dnsv1alpha1.DNSEntrySpec{
			DNSName: dnsName,
			TTL:     ptr.To(int64(120)),
			Targets: initialTargets,
			Text:    initialText,
		},
	}

	t.Logf("Creating DNSEntry %s for %s", entryName, dnsName)
	if err := testConfig.vucClient.Create(ctx, dnsEntry); err != nil {
		t.Fatalf("Failed to create DNSEntry %s: %v", entryName, err)
	}

	t.Cleanup(func() {
		t.Logf("Deleting DNSEntry %s", entryName)
		if err := testConfig.vucClient.Delete(ctx, dnsEntry); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("Failed to delete DNSEntry %s: %v", entryName, err)
			return
		}

		entryList := &dnsv1alpha1.DNSEntryList{}
		err := kubernetes.WaitForCondition[*dnsv1alpha1.DNSEntry](
			ctx, 2*time.Minute,
			func() (watch.Interface, error) {
				return testConfig.vucClient.Watch(ctx, entryList, client.InNamespace(testConfig.namespace), client.MatchingFields{"metadata.name": entryName})
			},
			func(_ *dnsv1alpha1.DNSEntry) bool {
				err := testConfig.vucClient.Get(ctx, client.ObjectKey{Namespace: testConfig.namespace, Name: entryName}, &dnsv1alpha1.DNSEntry{})
				return apierrors.IsNotFound(err)
			},
		)
		if err != nil {
			t.Errorf("DNSEntry %s was not deleted in time: %v", entryName, err)
		}
	})

	t.Logf("Waiting for DNSEntry %s to become Ready", entryName)
	entryList := &dnsv1alpha1.DNSEntryList{}
	err := kubernetes.WaitForCondition[*dnsv1alpha1.DNSEntry](
		ctx, 2*time.Minute,
		func() (watch.Interface, error) {
			return testConfig.vucClient.Watch(ctx, entryList, client.InNamespace(testConfig.namespace), client.MatchingFields{"metadata.name": entryName})
		},
		func(e *dnsv1alpha1.DNSEntry) bool {
			return e.Status.State == statusReady
		},
	)
	if err != nil {
		var e dnsv1alpha1.DNSEntry
		_ = testConfig.vucClient.Get(ctx, client.ObjectKey{Namespace: testConfig.namespace, Name: entryName}, &e)
		t.Fatalf("DNSEntry %s did not become Ready: %v (Status: %+v)", entryName, err, e.Status)
	}
	t.Logf("DNSEntry %s is Ready!", entryName)

	rrSetName := fmt.Sprintf("%s-%s", strings.ToLower(recordType), dnsName)
	t.Logf("Verifying ResourceRecordSet %s on GDC Global Cluster in namespace %s", rrSetName, testConfig.gdchProject)

	rrSetList := &globalnetworkingv1.ResourceRecordSetList{}
	var createdRRSet *globalnetworkingv1.ResourceRecordSet
	err = kubernetes.WaitForCondition[*globalnetworkingv1.ResourceRecordSet](
		ctx, 2*time.Minute,
		func() (watch.Interface, error) {
			return testConfig.globalClient.Watch(ctx, rrSetList, client.InNamespace(testConfig.gdchProject), client.MatchingFields{"metadata.name": rrSetName})
		},
		func(rr *globalnetworkingv1.ResourceRecordSet) bool {
			for _, cond := range rr.Status.Conditions {
				if cond.Type == statusReady && cond.Status == metav1.ConditionTrue {
					createdRRSet = rr
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf("ResourceRecordSet %s was not found or Ready in GDC: %v", rrSetName, err)
	}

	if diff := cmp.Diff(expectedInitial, createdRRSet.Spec.RRData); diff != "" {
		t.Errorf("ResourceRecordSet %s initial RRData mismatch (-want +got):\n%s", rrSetName, diff)
	}
	t.Logf("Successfully verified initial ResourceRecordSet %s on GDC!", rrSetName)

	t.Logf("Updating DNSEntry %s with new data", entryName)
	var currentEntry dnsv1alpha1.DNSEntry
	if err := testConfig.vucClient.Get(ctx, client.ObjectKey{Namespace: testConfig.namespace, Name: entryName}, &currentEntry); err != nil {
		t.Fatalf("Failed to get current DNSEntry %s for update: %v", entryName, err)
	}

	currentEntry.Spec.Targets = updatedTargets
	currentEntry.Spec.Text = updatedText
	if err := testConfig.vucClient.Update(ctx, &currentEntry); err != nil {
		t.Fatalf("Failed to update DNSEntry %s: %v", entryName, err)
	}

	t.Logf("Waiting for ResourceRecordSet %s on GDC to reflect updated data", rrSetName)
	var updatedRRSet *globalnetworkingv1.ResourceRecordSet
	err = kubernetes.WaitForCondition[*globalnetworkingv1.ResourceRecordSet](
		ctx, 2*time.Minute,
		func() (watch.Interface, error) {
			return testConfig.globalClient.Watch(ctx, rrSetList, client.InNamespace(testConfig.gdchProject), client.MatchingFields{"metadata.name": rrSetName})
		},
		func(rr *globalnetworkingv1.ResourceRecordSet) bool {
			if cmp.Equal(rr.Spec.RRData, expectedUpdated) {
				for _, cond := range rr.Status.Conditions {
					if cond.Type == statusReady && cond.Status == metav1.ConditionTrue {
						updatedRRSet = rr
						return true
					}
				}
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf("ResourceRecordSet %s did not update in time on GDC: %v", rrSetName, err)
	}

	if diff := cmp.Diff(expectedUpdated, updatedRRSet.Spec.RRData); diff != "" {
		t.Errorf("ResourceRecordSet %s updated RRData mismatch (-want +got):\n%s", rrSetName, diff)
	}
	t.Logf("Successfully verified updated ResourceRecordSet %s on GDC!", rrSetName)
}

func verifyPodHealth(ctx context.Context, t *testing.T, testConfig *config) {
	t.Helper()
	t.Log("Checking dns-controller-manager pod health and logs")

	podList := &corev1.PodList{}
	err := testConfig.vucClient.List(ctx, podList, client.InNamespace(testConfig.namespace), client.MatchingLabels(testConfig.dnsDeploymentLabel))
	if err != nil || len(podList.Items) == 0 {
		t.Fatalf("Failed to list pods for health check: %v", err)
	}

	for _, pod := range podList.Items {
		for _, status := range pod.Status.ContainerStatuses {
			if status.RestartCount > 0 {
				t.Errorf("Container %s in pod %s restarted %d times (expected 0). Termination reason: %v",
					status.Name, pod.Name, status.RestartCount, status.LastTerminationState)
			}
		}

		eventList := &corev1.EventList{}
		fieldSelector := client.MatchingFields{
			"involvedObject.name":      pod.Name,
			"involvedObject.namespace": testConfig.namespace,
			"involvedObject.kind":      "Pod",
		}
		if err := testConfig.vucClient.List(ctx, eventList, client.InNamespace(testConfig.namespace), fieldSelector); err != nil {
			t.Errorf("Failed to list events for pod %s: %v", pod.Name, err)
			continue
		}

		for _, event := range eventList.Items {
			if event.Type == corev1.EventTypeWarning && event.Reason == "Unhealthy" {
				t.Errorf("Pod %s experienced health probe failure: %s", pod.Name, event.Message)
			}
		}
	}
	t.Log("Pod health check passed: 0 restarts and 0 probe failures!")
}
