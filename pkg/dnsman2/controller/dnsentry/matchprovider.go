// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"strings"

	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

type providerMatch struct {
	found *v1alpha1.DNSProvider
	match int
}

func findBestMatchingProvider(providers []v1alpha1.DNSProvider, dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	handleMatch := func(match *providerMatch, p *v1alpha1.DNSProvider, n int, err error) error {
		if match.match > n {
			return err
		}
		if match.match < n || ptr.Deref(currentProviderName, "") == p.Name {
			match.found = p
			match.match = n
			return nil
		}
		return err
	}
	var err error
	validMatch := &providerMatch{}
	errorMatch := &providerMatch{}
	for _, p := range providers {
		n := matchSelection(dnsName, p.Status.Domains)
		if n > 0 {
			if p.Status.State == v1alpha1.StateReady {
				err = handleMatch(validMatch, &p, n, err)
			} else {
				err = handleMatch(errorMatch, &p, n, err)
			}
		}
	}
	if validMatch.found != nil {
		return validMatch.found, nil
	}
	if errorMatch.found != nil {
		return errorMatch.found, nil
	}
	return nil, err
}

func matchSelection(name string, selection v1alpha1.DNSSelectionStatus) int {
	ilen := matchDomains(name, selection.Included)
	elen := matchDomains(name, selection.Excluded)
	if ilen > elen {
		return ilen
	}
	return 0
}

func matchDomains(name string, domains []string) int {
	length := 0
	for _, d := range domains {
		if len(d) > length && matchesSuffix(name, d) {
			length = len(d)
		}
	}
	return length
}

func matchesSuffix(name, suffix string) bool {
	return name == suffix || strings.HasSuffix(name, "."+suffix)
}
