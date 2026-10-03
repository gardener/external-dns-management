// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gdcloud

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var gdcloudLocation = func() string {
	if loc := os.Getenv("GDCLOUD_PATH"); loc != "" {
		if info, err := os.Stat(loc); err == nil && info.IsDir() {
			return filepath.Join(loc, "bin", "gdcloud")
		}
		return loc
	}
	return "gdcloud"
}

// TestingClient provides a hermetic wrapper around the gdcloud CLI.
// It manages an isolated execution environment for safe concurrent testing.
type TestingClient struct {
	configDir string
}

// ConfigDir returns the path to the hermetic configuration directory managed by this client.
func (c *TestingClient) ConfigDir() string {
	return c.configDir
}

// NewTestingClient initializes a new hermetic gdcloud CLI client.
// It creates a temporary directory for gdcloud configuration and configures it with
// the provided CA data path, service account key file path, and console URL.
func NewTestingClient(caDataPath, serviceaccountPath string, consoleURL string) (client *TestingClient, err error) {
	// Create a temporary HOME directory to ensure gdcloud does not mutate the user's ~/.config/gdcloud
	configDir, err := os.MkdirTemp("", "gdcloud-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory for gdcloud config: %w", err)
	}

	defer func() {
		if err != nil {
			_ = os.RemoveAll(configDir)
		}
	}()

	c := &TestingClient{configDir: configDir}

	if result, err := c.Exec("config", "set", "auth/login_config_cert_path", caDataPath); err != nil {
		return nil, fmt.Errorf("%s, cannot execute gdcloud command: %w", result, err)
	}
	if result, err := c.Exec("config", "set", "core/organization_console_url", consoleURL); err != nil {
		return nil, fmt.Errorf("%s, cannot execute gdcloud command: %w", result, err)
	}
	if result, err := c.Exec("auth", "activate-service-account", "--key-file="+serviceaccountPath); err != nil {
		return nil, fmt.Errorf("%s, cannot execute gdcloud command: %w", result, err)
	}
	return c, nil
}

// Cleanup removes the temporary configuration directory used by the testing client.
func (c *TestingClient) Cleanup() error {
	return os.RemoveAll(c.configDir)
}

// Exec executes a gdcloud CLI command using the testing client's hermetic configuration.
// It locates the gdcloud binary and runs the command with an isolated HOME directory to prevent global side effects.
func (c *TestingClient) Exec(args ...string) ([]byte, error) {
	loc := gdcloudLocation()
	if loc == "" {
		return nil, fmt.Errorf("cannot locate gdcloud binary")
	}
	executable, err := exec.LookPath(loc)
	if err != nil {
		return nil, fmt.Errorf("cannot locate gdcloud binary: %w", err)
	}
	cmd := exec.Command(executable, args...) // #nosec G204 -- test helper executing gdcloud CLI
	// Override HOME and XDG_CONFIG_HOME to guarantee hermetic testing.
	// This forces gdcloud to write its configuration to our isolated test directory
	// instead of mutating the global ~/.config/gdcloud state. We override both
	// because some processes/libraries rigidly default to HOME while others
	// adhere strictly to XDG_CONFIG_HOME. exec.Cmd automatically uses the
	// last occurrence of duplicate environment keys.
	cmd.Env = append(os.Environ(),
		"HOME="+c.configDir,
		"XDG_CONFIG_HOME="+c.configDir,
	)

	result, err := cmd.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("cannot execute gdcloud command: %w", err)
	}
	return result, nil
}
