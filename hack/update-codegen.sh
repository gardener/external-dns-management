#!/usr/bin/env bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

PROJECT_ROOT=""$(readlink -f "$(dirname ${0})/..")""
source "${PROJECT_ROOT}/build/settings.src"

CODEGEN_PKG=${CODEGEN_PKG:-$(ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

rm -rf "${PROJECT_ROOT}/pkg/client/$APINAME"

# Go modules are not able to properly vendor k8s.io/code-generator@kubernetes-1.14.4 (build with godep)
# to include also generate-groups.sh scripts under vendor/.
# However this is fixed with kubernetes-1.15.0 (k8s.io/code-generator@kubernetes-1.15.0 is opted in go modules).
# The workaround for now is to have a kubernetes-1.14.4 copy of the script under hack/code-generator and
# to copy it to vendor/k8s.io/code-generator/ for the generation.
# And respectively clean up it after execution.
# The workaround should be removed adopting kubernetes-1.15.0.
# Similar thing is also done in https://github.com/heptio/contour/pull/1010.

cp "${PROJECT_ROOT}"/hack/code-generator/generate-groups.sh "${PROJECT_ROOT}"/vendor/k8s.io/code-generator/

cleanup() {
  rm -f "${PROJECT_ROOT}"/vendor/k8s.io/code-generator/generate-groups.sh
}
trap "cleanup" EXIT SIGINT

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
bash "${CODEGEN_PKG}/generate-groups.sh" "deepcopy,client,informer,lister" \
  $PKGPATH/pkg/client/$APINAME \
  $PKGPATH/pkg/apis \
  $APINAME:$APIVERSION \
  --go-header-file ${PROJECT_ROOT}/hack/custom-boilerplate.go.txt

# To use your own boilerplate text use:
#   --go-header-file ${PROJECT_ROOT}/hack/custom-boilerplate.go.txt
