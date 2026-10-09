// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/transport"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/gdcloud"
	"github.com/gardener/external-dns-management/test/integration/gdc/pkg/kubernetes"
)

type options struct {
	Scheme *runtime.Scheme
}

// Option is a function that configures the GDCH clients.
type Option func(*options)

// WithScheme provides an option to set a custom scheme for the clients.
func WithScheme(s *runtime.Scheme) Option {
	return func(opts *options) {
		opts.Scheme = s
	}
}

// Client wraps Kubernetes clients and the kubeconfig path for a GDC user cluster.
type Client struct {
	*kubernetes.Clients
	kubeconfigPath string
}

// GetKubeConfigPath returns the path to the generated kubeconfig file.
func (c *Client) GetKubeConfigPath() string {
	return c.kubeconfigPath
}

func getClient(url, caDataPath, serviceaccountPath string, opts ...Option) (client.WithWatch, error) {
	options := &options{
		Scheme: scheme.Scheme,
	}
	for _, opt := range opts {
		opt(options)
	}

	caData, err := os.ReadFile(caDataPath) // #nosec G304 -- test helper reading CA file
	if err != nil {
		return nil, fmt.Errorf("cannot read caData file: %w", err)
	}

	saJSON, err := os.ReadFile(serviceaccountPath) // #nosec G304 -- test helper reading SA file
	if err != nil {
		return nil, fmt.Errorf("cannot read serviceaccount file: %w", err)
	}

	var sa auth.ServiceAccount
	err = json.Unmarshal(saJSON, &sa)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal serviceAccountJSON: %w", err)
	}

	tokenSource := auth.NewCachedSTSTokenSource(url, &sa, auth.WithCACert(caData))

	config := &rest.Config{
		Host: url,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		WrapTransport: transport.TokenSourceWrapTransport(tokenSource),
	}

	k8sClient, err := client.NewWithWatch(config, client.Options{Scheme: options.Scheme})
	if err != nil {
		return nil, fmt.Errorf("create new client failure: %w", err)
	}
	return k8sClient, nil
}

// GetZonalClient returns a controller-runtime watch client for a GDC zonal management API server.
func GetZonalClient(org, zone, labURL, caDataPath, serviceaccountPath string, opts ...Option) (client.WithWatch, error) {
	url := fmt.Sprintf("https://management-kube.apiserver.%s.%s.%s", org, zone, labURL)
	return getClient(url, caDataPath, serviceaccountPath, opts...)
}

// GetGlobalClient returns a controller-runtime watch client for a GDC global API server.
func GetGlobalClient(org, zone, labURL, caDataPath, serviceaccountPath string, opts ...Option) (client.WithWatch, error) {
	url := fmt.Sprintf("https://global-api.%s.%s.%s", org, zone, labURL)
	return getClient(url, caDataPath, serviceaccountPath, opts...)
}

// GetUserClusterClient returns a client wrapper for a GDC user cluster.
func GetUserClusterClient(gdcloudClient *gdcloud.TestingClient, zone, project, clusterName string, opts ...Option) (*Client, error) {
	kubeConfigPath := filepath.Join(gdcloudClient.ConfigDir(), fmt.Sprintf("kubeconfig-%s-%s.yaml", zone, clusterName))
	if err := os.Setenv("KUBECONFIG", kubeConfigPath); err != nil {
		return nil, fmt.Errorf("unable to set KUBECONFIG variable: %w", err)
	}

	result, err := gdcloudClient.Exec("clusters", "get-credentials", clusterName, "--project="+project, "--zone="+zone, "--standard")
	if err != nil {
		return nil, fmt.Errorf("%s, cannot execute gdcloud command: %w", result, err)
	}
	// Inject HOME and XDG_CONFIG_HOME directly into the kubeconfig's exec plugin configuration.
	// This guarantees that when client-go spawns the gdcloud-k8s-auth-plugin subprocess,
	// the plugin inherits the isolated test directory path and doesn't fall back to the
	// user's global ~/.config/gdcloud directory, ensuring complete test hermeticity.
	if err := injectExecEnv(kubeConfigPath, gdcloudClient.ConfigDir()); err != nil {
		return nil, fmt.Errorf("failed to inject hermetic env into kubeconfig: %w", err)
	}

	options := &options{
		Scheme: scheme.Scheme,
	}
	for _, opt := range opts {
		opt(options)
	}

	k8sClients, err := kubernetes.NewClients(kubeConfigPath, kubernetes.WithScheme(options.Scheme))
	if err != nil {
		return nil, fmt.Errorf("cannot create k8s client: %w", err)
	}
	return &Client{
		Clients:        k8sClients,
		kubeconfigPath: kubeConfigPath,
	}, nil
}

func injectExecEnv(kubeConfigPath, configDir string) error {
	cfg, err := clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	for _, authInfo := range cfg.AuthInfos {
		if authInfo.Exec != nil {
			// Ensure the HOME and XDG_CONFIG_HOME env vars are passed to the exec plugin (e.g., gdcloud-k8s-auth-plugin)
			authInfo.Exec.Env = append(authInfo.Exec.Env,
				clientcmdapi.ExecEnvVar{Name: "HOME", Value: configDir},
				clientcmdapi.ExecEnvVar{Name: "XDG_CONFIG_HOME", Value: configDir},
			)
		}
	}

	kubeConfigBytes, err := clientcmd.Write(*cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize modified kubeconfig: %w", err)
	}
	if err := os.WriteFile(kubeConfigPath, kubeConfigBytes, 0600); err != nil {
		return fmt.Errorf("failed to write modified kubeconfig: %w", err)
	}
	return nil
}
