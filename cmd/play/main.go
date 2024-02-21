// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"

	"github.com/gardener/external-dns-management/pkg/controller/provider/alicloud"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

func main() {
	var r raw.Record

	ali := alidns.Record{Value: "test"}

	r = (*alicloud.Record)(&ali)

	fmt.Printf("Value: %s\n", r.GetValue())

	back := (*alidns.Record)(r.(*alicloud.Record))
	fmt.Printf("Value: %s\n", back.Value)
}
