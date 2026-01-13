// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
//
//
// This small tool can be used to create a given number of DNS entries for load tests:
//
// Usage:
//	go run main.go
//
//  Command line options:
//     --base-domain string
//     		base domain for the entries (mandatory)
//     --base-entry int
//     		base index for the entries (default 0, i.e. e00000, e00001, ...)
//     --count int
//     		number of entries to create (default 10)
//     --kubeconfig string
//     		absolute path to the kubeconfig file (defaults to the env variable `KUBECONFIG`)
//     --label string
//     		label value for label 'loadtest' to set on the entries (default "true")
//     --with-routing-policy=true
//     		entries should have a routing policy set (default false)
//     --latency duration
//     		latency between creation of two entries (default 0)
//     --ttl int
//     		TTL for the entries in seconds (default 120)
// You may use `kubectl delete dnsentry -l loadtest=<label-value>` to delete them all at once.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsmanclient "github.com/gardener/external-dns-management/pkg/dnsman2/client"
	"github.com/gardener/gardener/pkg/controllerutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	kubeconfig    string
	baseDomain    string
	baseEntry     int
	labelValue    string
	routingPolicy bool
	count         int
	latency       time.Duration
	ttl           int64
)

func main() {
	flag.StringVar(&kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "absolute path to the kubeconfig file")
	flag.IntVar(&count, "count", 10, "number of entries to create")
	flag.IntVar(&baseEntry, "base-entry", 0, "base index for the entries (default 0, i.e. e00000, e00001, ...)")
	flag.StringVar(&baseDomain, "base-domain", "", "base domain for the entries")
	flag.StringVar(&labelValue, "label", "true", "label value for label 'loadtest' to set on the entries")
	flag.BoolVar(&routingPolicy, "with-routing-policy", false, "entries should have a routing policy set (default false)")
	flag.DurationVar(&latency, "latency", 0, "latency between creation of two entries (default 0)")
	flag.Int64Var(&ttl, "ttl", 120, "TTL for the entries in seconds (default 120)")
	flag.Parse()

	if baseDomain == "" {
		fmt.Fprintf(os.Stderr, "-base-domain is required\n")
		os.Exit(1)
	}

	c, err := createClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create client: %w\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	fmt.Fprintf(os.Stdout, "Creating %d entries - please wait\n", count)

	if err := createEntries(ctx, c, count, "e%05d", baseDomain, "loadtest", labelValue); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create entries: %w\n", err)
		os.Exit(2)
	}

	fmt.Fprintf(os.Stdout, "Done - all %d entries created\n", count)
}

func createEntries(ctx context.Context, c client.Client, count int, nameTemplate, baseDomain, labelKey, labelValue string) error {
	for i := range count {
		name := fmt.Sprintf(nameTemplate, i+baseEntry)
		entry := &dnsv1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      name,
			},
		}
		if _, err := controllerutils.CreateOrGetAndMergePatch(ctx, c, entry, func() error {
			entry.Labels = map[string]string{
				labelKey: labelValue,
			}

			entry.Spec = dnsv1alpha1.DNSEntrySpec{
				DNSName: fmt.Sprintf("%s.%s", name, baseDomain),
				Targets: []string{fmt.Sprintf("2.%d.%d.%d", i>>16, (i&0xff00)>>8, i&0xff)},
				TTL:     ptr.To(ttl),
			}
			if routingPolicy {
				entry.Spec.RoutingPolicy = &dnsv1alpha1.RoutingPolicy{
					Type:          "weighted",
					SetIdentifier: "set1",
					Parameters: map[string]string{
						"weight": "100",
					},
				}
			} else {
				entry.Spec.RoutingPolicy = nil
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to create/update entry %s: %w", entry.Name, err)
		}
		if i > 0 && i%100 == 0 {
			fmt.Fprintf(os.Stdout, "%d/%d entries created...\n", i, count)
		}
		if latency > 0 {
			time.Sleep(latency)
		}
	}

	return nil
}

func createClient() (client.Client, error) {
	if kubeconfig == "" {
		return nil, fmt.Errorf("-kubeconfig or KUBECONFIG env var is required")
	}

	cfg, err := clientcmd.LoadFromFile(kubeconfig)
	if err != nil {
		return nil, err
	}
	clientConfig := clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return client.New(restConfig, client.Options{Scheme: dnsmanclient.ClusterScheme})
}
