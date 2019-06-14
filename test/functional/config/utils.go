package config

import (
	"encoding/json"
	"fmt"
	"github.com/onsi/gomega"
	"math/rand"
	"net"
	"os/exec"
	"strings"
	"time"
)

const STATE_DELETED = "~DELETED~"
const letterBytes = "abcdefghijklmnopqrstuvwxyz"

type TestUtils struct {
	AwaitTimeout  time.Duration
	LookupTimeout time.Duration
	PollingPeriod time.Duration
	Namespace     string
	Verbose       bool
}

func CreateDefaultTestUtils() *TestUtils {
	return &TestUtils{
		AwaitTimeout:  30 * time.Second,
		LookupTimeout: 420 * time.Second, // needed probably because of (too) long DNS caching settings at SAP(?)
		PollingPeriod: 100 * time.Millisecond,
		Namespace:     "default",
		Verbose:       true,
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
		println(string(err.(*exec.ExitError).Stderr))
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
		output, err := u.runKubeCtl("get " + resourceName + " \"-o=jsonpath={range .items[*]}{.metadata.name}={.status.state}{'\\n'}{end}\"")
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

type CheckFunc func() (bool, error)

func (u *TestUtils) Await(msg string, check CheckFunc) error {
	return u.AwaitWithTimeout(msg, check, u.AwaitTimeout)
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
	expectedAddrs, err := net.LookupHost(target)
	gomega.Ω(err).Should(gomega.BeNil())

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

	gomega.Eventually(func() error {
		_, err := lookup(dnsname)
		return err
	}, u.LookupTimeout, u.PollingPeriod).Should(gomega.BeNil())

	addrs, err := lookup(dnsname)

	gomega.Ω(err).Should(gomega.BeNil())
	gomega.Ω(addrs).Should(gomega.ConsistOf(itfs...))
}

func (u *TestUtils) AwaitLookup(dnsname string, expected ...string) {
	u.AwaitLookupFunc(net.LookupHost, dnsname, expected...)
}

func (u *TestUtils) AwaitLookupTXT(dnsname string, expected ...string) {
	u.AwaitLookupFunc(net.LookupTXT, dnsname, expected...)
}

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
