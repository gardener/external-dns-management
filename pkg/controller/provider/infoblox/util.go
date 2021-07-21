/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

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
