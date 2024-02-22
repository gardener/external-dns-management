// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/gomega"
)

const (
	STATE_DELETED = "~DELETED~"
	letterBytes   = "abcdefghijklmnopqrstuvwxyz"
)

type TestUtils struct {
	AwaitTimeout     time.Duration
	LookupTimeout    time.Duration
	PollingPeriod    time.Duration
	Namespace        string
	Verbose          bool
	dnsClient        *dnsClient
	nextAwaitTimeout time.Duration
}

func CreateDefaultTestUtils(dnsServer string) *TestUtils {
	return &TestUtils{
		AwaitTimeout:  30 * time.Second,
		LookupTimeout: 420 * time.Second, // needed probably because of (too) long DNS caching settings at SAP(?)
		PollingPeriod: 200 * time.Millisecond,
		Namespace:     "default",
		Verbose:       true,
		dnsClient:     createDNSClient(dnsServer),
	}
}

func (u *TestUtils) KubectlGetAllDNSEntries() (map[string]interface{}, error) {
	output, err := u.runKubeCtl("get dnse -o json")
	if err != nil {
		return nil, err
	}
	return u.toItemMap(output)
}

func (u *TestUtils) toItemMap(output string) (map[string]interface{}, error) {
	untyped := map[string]interface{}{}
	err := json.Unmarshal([]byte(output), &untyped)
	if err != nil {
		return nil, err
	}

	if untyped["kind"] != "List" {
		return nil, fmt.Errorf("Result is not a list")
	}

	itemMap := map[string]interface{}{}
	items := untyped["items"].([]interface{})
	for _, rawItem := range items {
		item := rawItem.(map[string]interface{})
		name := item["metadata"].(map[string]interface{})["name"].(string)
		itemMap[name] = item
	}
	return itemMap, err
}

func (u *TestUtils) KubectlApply(filename string) error {
	output, err := u.runKubeCtl(fmt.Sprintf("apply -f %q", filename))
	u.LogVerbose(output)
	return err
}

func (u *TestUtils) KubectlDelete(filename string) error {
	output, err := u.runKubeCtl(fmt.Sprintf("delete -f %q", filename))
	u.LogVerbose(output)
	return err
}

func (u *TestUtils) LogVerbose(output string) {
	if u.Verbose {
		println(output)
	}
}

func (u *TestUtils) runKubeCtl(cmdline string) (string, error) {
	return u.runCmd("kubectl -n " + u.Namespace + " " + cmdline)
}

func (u *TestUtils) runCmd(cmdline string) (string, error) {
	cmd := exec.Command("sh", "-c", cmdline)
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("command `%s` failed: %s\n", cmdline, string(err.(*exec.ExitError).Stderr))
		return string(out), fmt.Errorf("command `%s` failed with %s", cmdline, err)
	}
	return string(out), nil
}

func (u *TestUtils) AwaitDNSProviderReady(names ...string) error {
	return u.AwaitState("dnspr", "Ready", names...)
}

func (u *TestUtils) AwaitDNSProviderDeleted(names ...string) error {
	return u.AwaitState("dnspr", STATE_DELETED, names...)
}

func (u *TestUtils) AwaitDNSEntriesReady(names ...string) error {
	return u.AwaitState("dnse", "Ready", names...)
}

func (u *TestUtils) AwaitDNSEntriesError(names ...string) error {
	return u.AwaitState("dnse", "Error", names...)
}

func (u *TestUtils) AwaitDNSEntriesDeleted(names ...string) error {
	return u.AwaitState("dnse", STATE_DELETED, names...)
}

func (u *TestUtils) AwaitState(resourceName, expectedState string, names ...string) error {
	msg := fmt.Sprintf("%s not %s: %v", resourceName, expectedState, names)
	return u.Await(msg, func() (bool, error) {
		output, err := u.runKubeCtl("get " + resourceName + " \"-o=jsonpath={range .items[?(@.status.state)]}{.metadata.name}={.status.state}{'\\n'}{end}\"")
		if err != nil {
			return false, err
		}

		states := map[string]string{}
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			cols := strings.Split(line, "=")
			if len(cols) == 2 {
				states[cols[0]] = cols[1]
			}
		}
		for _, name := range names {
			if expectedState == STATE_DELETED {
				if _, ok := states[name]; ok {
					return false, nil
				}
			} else if states[name] != expectedState {
				return false, nil
			}
		}
		return true, nil
	})
}

func (u *TestUtils) SetTimeoutForNextAwait(timeout time.Duration) {
	u.nextAwaitTimeout = timeout
}

type CheckFunc func() (bool, error)

func (u *TestUtils) Await(msg string, check CheckFunc) error {
	timeout := u.AwaitTimeout
	if u.nextAwaitTimeout != 0 {
		timeout = u.nextAwaitTimeout
		u.nextAwaitTimeout = 0
	}
	return u.AwaitWithTimeout(msg, check, timeout)
}

func (u *TestUtils) AwaitWithTimeout(msg string, check CheckFunc, timeout time.Duration) error {
	var err error
	var ok bool

	limit := time.Now().Add(timeout)
	for time.Now().Before(limit) {
		ok, err = check()
		if ok {
			return nil
		}
		time.Sleep(u.PollingPeriod)
	}
	if err != nil {
		return fmt.Errorf("Timeout during check %s with error %s", msg, err.Error())
	}
	return fmt.Errorf("Timeout during check  %s", msg)
}

func (u *TestUtils) AwaitLookupCName(dnsname, target string) {
	expectedAddrs, err := u.dnsClient.LookupHost(target)
	gomega.Ω(err).Should(gomega.BeNil(), "Cannot lookup CNAME "+target)

	u.AwaitLookup(dnsname, expectedAddrs...)
}

type LookupFunc func(dnsname string) ([]string, error)

func toIfts(names []string) []interface{} {
	itfs := []interface{}{}
	for _, name := range names {
		itfs = append(itfs, name)
	}
	return itfs
}

func (u *TestUtils) AwaitLookupFunc(lookup LookupFunc, dnsname string, expected ...string) {
	u.LogVerbose(fmt.Sprintf("DNS lookup for %s...\n", dnsname))

	itfs := toIfts(expected)

	var addrs []string
	var err error
	gomega.Eventually(func() error {
		addrs, err = lookup(dnsname)
		return err
	}, u.LookupTimeout, u.PollingPeriod).Should(gomega.BeNil())

	gomega.Ω(err).Should(gomega.BeNil())
	gomega.Ω(addrs).Should(gomega.ConsistOf(itfs...))
}

func (u *TestUtils) AwaitLookup(dnsname string, expected ...string) {
	u.AwaitLookupFunc(u.dnsClient.LookupHost, dnsname, expected...)
}

func (u *TestUtils) AwaitLookupTXT(dnsname string, expected ...string) {
	u.AwaitLookupFunc(u.dnsClient.LookupTXT, dnsname, expected...)
}

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func (u *TestUtils) CanLookup(privateDNS bool) bool {
	if u.dnsClient.client == nil {
		return true
	}
	return !privateDNS
}

func (u *TestUtils) AwaitKubectlGetCRDs(crds ...string) error {
	var err error
	for _, crd := range crds {
		gomega.Eventually(func() error {
			_, err = u.runKubeCtl("get crd " + crd)
			return err
		}, u.AwaitTimeout, u.PollingPeriod).Should(gomega.BeNil())
		if err != nil {
			return err
		}
	}
	return err
}
