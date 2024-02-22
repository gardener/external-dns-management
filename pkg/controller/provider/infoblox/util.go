// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package infoblox

import (
	"net/http"
	"strconv"

	ibclient "github.com/infobloxopen/infoblox-go-client/v2"
)

// MaxResultsRequestBuilder implements a HttpRequestBuilder which sets the
// _max_results query parameter on all get requests
type MaxResultsRequestBuilder struct {
	maxResults string
	ibclient.HttpRequestBuilder
}

// NewMaxResultsRequestBuilder returns a MaxResultsRequestBuilder which adds
// _max_results query parameter to all GET requests
func NewMaxResultsRequestBuilder(maxResults int, requestBuilder ibclient.HttpRequestBuilder) ibclient.HttpRequestBuilder {
	return &MaxResultsRequestBuilder{
		maxResults:         strconv.Itoa(maxResults),
		HttpRequestBuilder: requestBuilder,
	}
}

// BuildRequest prepares the api request. it uses BuildRequest of
// WapiRequestBuilder and then add the _max_requests parameter
func (mrb *MaxResultsRequestBuilder) BuildRequest(t ibclient.RequestType, obj ibclient.IBObject, ref string, queryParams *ibclient.QueryParams) (req *http.Request, err error) {
	req, err = mrb.HttpRequestBuilder.BuildRequest(t, obj, ref, queryParams)
	if req.Method == "GET" {
		query := req.URL.Query()
		query.Set("_max_results", mrb.maxResults)
		req.URL.RawQuery = query.Encode()
	}
	return
}
