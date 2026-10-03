// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"sigs.k8s.io/yaml"
)

// InstallOptions contains the options for installing a Helm chart.
type InstallOptions struct {
	ChartPath      string
	KubeconfigPath string
	ReleaseName    string
	Namespace      string
	Values         map[string]interface{}
	Timeout        time.Duration
}

// UninstallOptions contains the options for uninstalling a Helm chart.
type UninstallOptions struct {
	KubeconfigPath string
	ReleaseName    string
	Namespace      string
	IgnoreNotFound bool
	// Wait will wait for all the resources to be deleted.
	Wait bool
}

// InstallOrUpgrade installs or upgrades a Helm chart using the helm CLI.
func InstallOrUpgrade(opts InstallOptions) error {
	valuesBytes, err := yaml.Marshal(opts.Values)
	if err != nil {
		return fmt.Errorf("failed to marshal helm values: %w", err)
	}

	valuesFile, err := os.CreateTemp("", "helm-values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp values file: %w", err)
	}
	defer func() {
		_ = os.Remove(valuesFile.Name())
	}()

	if _, err := valuesFile.Write(valuesBytes); err != nil {
		_ = valuesFile.Close()
		return fmt.Errorf("failed to write helm values file: %w", err)
	}
	if err := valuesFile.Close(); err != nil {
		return fmt.Errorf("failed to close helm values file: %w", err)
	}

	args := []string{
		"upgrade",
		"--install",
		"--create-namespace",
		"--kubeconfig", opts.KubeconfigPath,
		"--namespace", opts.Namespace,
		"--values", valuesFile.Name(),
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", opts.Timeout.String())
	}
	args = append(args, opts.ReleaseName, opts.ChartPath)

	cmd := exec.Command("helm", args...) // #nosec G204 -- test helper executing helm CLI
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run helm upgrade --install (%s): %w", string(output), err)
	}
	return nil
}

// Uninstall removes a specified Helm release from a cluster using the helm CLI.
func Uninstall(opts UninstallOptions) error {
	args := []string{
		"uninstall",
		"--kubeconfig", opts.KubeconfigPath,
		"--namespace", opts.Namespace,
	}
	if opts.IgnoreNotFound {
		args = append(args, "--ignore-not-found")
	}
	if opts.Wait {
		args = append(args, "--wait")
	}
	args = append(args, opts.ReleaseName)

	cmd := exec.Command("helm", args...) // #nosec G204 -- test helper executing helm CLI
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to uninstall release %q in namespace %q (%s): %w", opts.ReleaseName, opts.Namespace, string(output), err)
	}
	return nil
}
