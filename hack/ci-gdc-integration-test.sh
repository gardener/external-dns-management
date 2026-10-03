#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

COMMIT_HASH="${COMMIT_HASH:-$(git rev-parse --short HEAD)}"
ZONE="${GDC_ZONE:-us-west6-a}"
GDCH_PROJECT="${GDC_PROJECT:-sapbtp}"
VUC="${GDC_VUC:-gardener-github-ci}"
ORG="${GDC_ORG:-gdc1}"
LAB_URL="${GDC_LAB_URL:-staging.gpcdemolabs.com}"
MANAGED_DNS_ZONE="${GDC_MANAGED_DNS_ZONE:-sap.gardener.gpcdemolabs.com}"
GDCLOUD_VERSION="${GDCLOUD_VERSION:-1.16.2}"
SHOOT_DNS_SERVICE_TAG="${SHOOT_DNS_SERVICE_TAG:-v1.83.0}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-ghcr.io/gardener/external-dns-management/dns-controller-manager}"
IMAGE_TAG="${IMAGE_TAG:-pr-${COMMIT_HASH}}"
IMAGE_WITHTAG="${IMAGE_REPOSITORY}:${IMAGE_TAG}"

WORK_DIR="$(mktemp -d)"
CA_FILE="${WORK_DIR}/cafile"
SA_FILE="${GDC_SERVICE_ACCOUNT_FILE:-${WORK_DIR}/gdc_service_account.json}"
TOKEN_HELPER_DIR="${WORK_DIR}/token-helper"
MGMT_URL="https://management-kube.apiserver.${ORG}.${ZONE}.${LAB_URL}"

cleanup() {
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT INT TERM

should_run_gdc_tests() {
  if [[ "${FORCE_RUN:-false}" == "true" ]]; then
    return 0
  fi

  if [[ -z "${BASE_REF:-}" ]] || ! git rev-parse --verify "${BASE_REF}" >/dev/null 2>&1; then
    return 0
  fi

  local changed_files
  changed_files=$(git diff --name-only "${BASE_REF}...HEAD" || true)
  if [[ -z "${changed_files}" ]]; then
    return 1
  fi

  while IFS= read -r file; do
    [[ -z "${file}" ]] && continue
    case "${file}" in
      *.md)
        ;;
      pkg/controller/provider/gdc/*|\
      pkg/dnsman2/dns/provider/handler/gdc/*|\
      examples/*gdc*|\
      test/integration/gdc/*|\
      hack/ci-gdc-integration-test.sh|\
      .github/workflows/gdc-integration-test.yaml)
        return 0
        ;;
    esac
  done <<< "${changed_files}"

  return 1
}

if ! should_run_gdc_tests; then
  echo "No GDC-specific code changes detected between ${BASE_REF:-} and HEAD. Skipping GDC integration test."
  exit 0
fi

if [[ -n "${GDC_SERVICE_ACCOUNT_KEY:-}" ]]; then
  umask 077
  printf '%s' "${GDC_SERVICE_ACCOUNT_KEY}" > "${SA_FILE}"
fi

if [[ ! -s "${SA_FILE}" ]]; then
  echo "Error: GDC Service Account key is required (set GDC_SERVICE_ACCOUNT_KEY or GDC_SERVICE_ACCOUNT_FILE)." >&2
  exit 1
fi

echo "Fetching GDC Root CA from console.${ORG}.${ZONE}.${LAB_URL}..."
wget "https://console.${ORG}.${ZONE}.${LAB_URL}/.well-known/certificate-authority" \
  --no-check-certificate -q -O "${CA_FILE}"

install_gdcloud_cli() {
  local gdcloud_root="/usr/local/bin/google-distributed-cloud-hosted-cli"
  if [[ -x "${gdcloud_root}/bin/gdcloud" && -x "${gdcloud_root}/bin/gdcloud-k8s-auth-plugin" ]]; then
    echo "gdcloud CLI already installed at ${gdcloud_root}/bin/gdcloud"
    export PATH="${gdcloud_root}/bin:${PATH}"
    export GDCLOUD_PATH="${gdcloud_root}/bin/gdcloud"
    return 0
  fi

  echo "Minting STS token using GDC ServiceAccount to query CLIBundleMetadata..."
  mkdir -p "${TOKEN_HELPER_DIR}"
  cat << 'EOF' > "${TOKEN_HELPER_DIR}/main.go"
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
)

func main() {
	saBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var sa auth.ServiceAccount
	if err := json.Unmarshal(saBytes, &sa); err != nil {
		panic(err)
	}
	caBytes, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	ts := auth.NewSTSTokenSource(os.Args[3], &sa, auth.WithCACert(caBytes))
	tok, err := ts.Token()
	if err != nil {
		panic(err)
	}
	fmt.Print(tok.AccessToken)
}
EOF

  local sts_token
  sts_token=$(go run "${TOKEN_HELPER_DIR}/main.go" "${SA_FILE}" "${CA_FILE}" "${MGMT_URL}")

  echo "Resolving gdcloud CLI bundle URL (version: ${GDCLOUD_VERSION}) from ${MGMT_URL}..."
  local serving_url
  serving_url=$(curl -ksSL -H "Authorization: Bearer ${sts_token}" \
    "${MGMT_URL}/apis/artifactview.private.gdc.goog/v1alpha1/namespaces/ui-system/clibundlemetadata" | \
    jq -r --arg ver "${GDCLOUD_VERSION}" '
      [.items[]
       | select(.platform.os == "linux" and .platform.architecture == "amd64")
       | select($ver == "" or (.commonMetadata.artifactVersion | contains($ver)))
      ]
      | sort_by(.metadata.creationTimestamp)
      | last
      | .commonMetadata.servingURL // empty
    ')

  if [[ -z "${serving_url}" ]]; then
    echo "Error: Failed to resolve gdcloud CLI servingURL for version '${GDCLOUD_VERSION}'." >&2
    exit 1
  fi

  local tarball="/tmp/gdcloud_cli_linux.tar.gz"
  echo "Downloading gdcloud CLI from ${serving_url}..."
  curl -kL --fail --retry 3 "${serving_url}?uncompressed=false" -o "${tarball}"

  local sudo_cmd=""
  if [[ "${EUID:-$(id -u)}" -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
    sudo_cmd="sudo"
  fi

  echo "Extracting gdcloud CLI to /usr/local/bin and installing gdcloud-k8s-auth-plugin..."
  ${sudo_cmd} tar -xzf "${tarball}" -C /usr/local/bin/
  ${sudo_cmd} "${gdcloud_root}/bin/gdcloud" components install gdcloud-k8s-auth-plugin
  rm -f "${tarball}"

  export PATH="${gdcloud_root}/bin:${PATH}"
  export GDCLOUD_PATH="${gdcloud_root}/bin/gdcloud"
  "${GDCLOUD_PATH}" version
}

install_gdcloud_cli

echo "Cloning gardener-extension-shoot-dns-service (${SHOOT_DNS_SERVICE_TAG}) and packaging chart..."
SHOOT_DNS_DIR="${WORK_DIR}/gardener-extension-shoot-dns-service"
git clone --depth 1 --branch "${SHOOT_DNS_SERVICE_TAG}" \
  https://github.com/gardener/gardener-extension-shoot-dns-service.git "${SHOOT_DNS_DIR}"
rm -f "${SHOOT_DNS_DIR}"/charts/gardener-extension-shoot-dns-service/templates/*logging.yaml
CHART_VERSION=$(yq '.version' "${SHOOT_DNS_DIR}/charts/gardener-extension-shoot-dns-service/Chart.yaml")
helm package "${SHOOT_DNS_DIR}/charts/gardener-extension-shoot-dns-service" --destination "${WORK_DIR}"
CHART_PACKAGE="${WORK_DIR}/gardener-extension-shoot-dns-service-${CHART_VERSION}.tgz"

echo "Building and pushing dns-controller-manager image ${IMAGE_WITHTAG}..."
docker build -t "${IMAGE_WITHTAG}" -f Dockerfile --target dns-controller-manager .
docker push "${IMAGE_WITHTAG}"

echo "Running GDC external-dns-management presubmit integration test..."
CGO_ENABLED=0 go test -v -timeout=20m ./test/integration/gdc \
  -args \
  --commit_hash="${COMMIT_HASH}" \
  --zone="${ZONE}" \
  --project="${GDCH_PROJECT}" \
  --vuc="${VUC}" \
  --org="${ORG}" \
  --lab_url="${LAB_URL}" \
  --cafile="${CA_FILE}" \
  --service_account="${SA_FILE}" \
  --image_tag="${IMAGE_WITHTAG}" \
  --chart_package="${CHART_PACKAGE}" \
  --managed_dns_zone="${MANAGED_DNS_ZONE}"
