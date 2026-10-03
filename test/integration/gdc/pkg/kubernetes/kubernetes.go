// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Clients holds Kubernetes clients for CRUD, Watch, and standard client-go operations.
type Clients struct {
	// WatchClient is the high-level controller-runtime client for CRUD and Watch.
	WatchClient client.WithWatch
	// Client is the standard client-go clientset for special actions like Exec.
	Client *kubernetes.Clientset
	// Config is the REST configuration.
	Config *rest.Config
}

type newClientsOptions struct {
	Scheme *runtime.Scheme
}

// newClientsOption is a function that configures the Clients.
type newClientsOption func(*newClientsOptions)

// WithScheme provides an option to set a custom scheme for the clients.
func WithScheme(s *runtime.Scheme) newClientsOption {
	return func(opts *newClientsOptions) {
		opts.Scheme = s
	}
}

// NewClients creates a new Clients bundle from a kubeconfig file path.
func NewClients(kubeconfigPath string, opts ...newClientsOption) (*Clients, error) {
	options := &newClientsOptions{
		Scheme: scheme.Scheme,
	}

	for _, opt := range opts {
		opt(options)
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("could not build config: %w", err)
	}

	watchClient, err := client.NewWithWatch(config, client.Options{Scheme: options.Scheme})
	if err != nil {
		return nil, fmt.Errorf("could not create controller-runtime client: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create client-go clientset: %w", err)
	}

	return &Clients{
		WatchClient: watchClient,
		Client:      clientset,
		Config:      config,
	}, nil
}

// CreateNamespace creates a new namespace.
func CreateNamespace(ctx context.Context, k8sclient client.WithWatch, name string) error {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if err := k8sclient.Create(ctx, namespaceObj); err != nil {
		return fmt.Errorf("failed to create Namespace: %w", err)
	}
	return nil
}

// CleanupResources cascades deletion of a test namespace.
func CleanupResources(t *testing.T, k8sclient client.WithWatch, namespace string) {
	t.Helper()

	ctx := context.Background()
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	t.Logf("Cleaning up resources in namespace %s", namespace)
	if err := k8sclient.Delete(ctx, namespaceObj); err != nil && !apierrors.IsNotFound(err) {
		t.Logf("Failed to cascade deleting resources in namespace %q: %v", namespace, err)
	} else {
		t.Logf("Cascade deleted all the resources in namespace %q", namespace)
	}
}

// WaitForPodReady waits for a Pod to reach 'Running' phase and 'Ready' condition.
func WaitForPodReady(ctx context.Context, k8sclient client.WithWatch, namespace, podName string, timeout time.Duration) error {
	podList := &corev1.PodList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{"metadata.name": podName},
	}

	err := WaitForCondition[*corev1.Pod](
		ctx, timeout,
		func() (watch.Interface, error) {
			return k8sclient.Watch(ctx, podList, listOptions...)
		},
		func(pod *corev1.Pod) bool {
			if pod.Status.Phase != corev1.PodRunning {
				return false
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					return true
				}
			}
			return false
		},
	)
	if err != nil {
		return fmt.Errorf("Pod %s was not Ready: %v", podName, err)
	}
	return nil
}

// WaitForDeploymentReady waits for a Deployment's 'Available' condition to be 'True'.
func WaitForDeploymentReady(ctx context.Context, k8sclient client.WithWatch, namespace, deploymentName string, timeout time.Duration) error {
	deploymentList := &appsv1.DeploymentList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingFields{"metadata.name": deploymentName},
	}

	err := WaitForCondition[*appsv1.Deployment](
		ctx, timeout,
		func() (watch.Interface, error) {
			return k8sclient.Watch(ctx, deploymentList, listOptions...)
		},
		func(deployment *appsv1.Deployment) bool {
			// First, ensure the controller has observed the latest generation of the spec
			if deployment.Status.ObservedGeneration < deployment.Generation {
				return false
			}

			// Check if all replicas are updated and available
			replicas := ptr.Deref(deployment.Spec.Replicas, 1)
			if deployment.Status.UpdatedReplicas == replicas &&
				deployment.Status.AvailableReplicas == replicas {
				return true
			}

			return false
		},
	)
	if err != nil {
		return fmt.Errorf("deployment %s was not ready: %w", deploymentName, err)
	}
	return nil
}

// WaitForCondition watches a Kubernetes resource until the isReady predicate returns true
// or the timeout is reached.
func WaitForCondition[T runtime.Object](
	ctx context.Context,
	timeout time.Duration,
	startWatch func() (watch.Interface, error),
	isReady func(obj T) bool,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var watcher watch.Interface
	defer func() {
		if watcher != nil {
			watcher.Stop()
		}
	}()

	for {
		if watcher == nil {
			var err error
			watcher, err = startWatch()
			if err != nil {
				select {
				case <-ctx.Done():
					return fmt.Errorf("failed to start watch: %w", err)
				case <-time.After(2 * time.Second):
					continue
				}
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition")
		case event, ok := <-watcher.ResultChan():
			if !ok {
				watcher.Stop()
				watcher = nil
				if ctx.Err() != nil {
					return fmt.Errorf("timeout waiting for condition")
				}
				continue
			}
			obj, ok := event.Object.(T)
			if !ok {
				continue
			}
			if isReady(obj) {
				return nil
			}
		}
	}
}
