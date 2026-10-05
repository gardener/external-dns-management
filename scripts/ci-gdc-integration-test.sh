#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

# shellcheck source=scripts/ci-gdc-common.sh
source "${REPO_ROOT}/scripts/ci-gdc-common.sh"

check_ci_preconditions

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
MGMT_URL="https://management-kube.apiserver.${ORG}.${ZONE}.${LAB_URL}"
IMAGE_PUSHED=false

cleanup() {
  local exit_code=$?
  if [[ "${IMAGE_PUSHED}" == "true" ]]; then
    delete_ghcr_image_tag "${IMAGE_REPOSITORY}" "${IMAGE_TAG}"
  fi
  rm -rf "${WORK_DIR}"
  complete_pr_check_run "${exit_code}"
}
trap cleanup EXIT INT TERM

setup_gdc_credentials "${SA_FILE}" "${CA_FILE}" "${ORG}" "${ZONE}" "${LAB_URL}" "${GDC_DNS_SERVICE_ACCOUNT_KEY:-}"
unset GDC_DNS_SERVICE_ACCOUNT_KEY
install_gdcloud_cli "${SA_FILE}" "${CA_FILE}" "${MGMT_URL}" "${GDCLOUD_VERSION}" "./test/integration/gdc/cmd/token-helper"

echo "Cloning gardener-extension-shoot-dns-service (${SHOOT_DNS_SERVICE_TAG}) and packaging chart..."
SHOOT_DNS_DIR="${WORK_DIR}/gardener-extension-shoot-dns-service"
git clone --depth 1 --branch "${SHOOT_DNS_SERVICE_TAG}" \
  https://github.com/gardener/gardener-extension-shoot-dns-service.git "${SHOOT_DNS_DIR}"
rm -f "${SHOOT_DNS_DIR}"/charts/gardener-extension-shoot-dns-service/templates/*logging.yaml
CHART_VERSION=$(yq '.version' "${SHOOT_DNS_DIR}/charts/gardener-extension-shoot-dns-service/Chart.yaml")
helm package "${SHOOT_DNS_DIR}/charts/gardener-extension-shoot-dns-service" --destination "${WORK_DIR}"
CHART_PACKAGE="${WORK_DIR}/gardener-extension-shoot-dns-service-${CHART_VERSION}.tgz"

echo "Building and pushing dns-controller-manager image ${IMAGE_WITHTAG}..."
retry_cmd docker build -t "${IMAGE_WITHTAG}" -f Dockerfile --target dns-controller-manager .
retry_cmd docker push "${IMAGE_WITHTAG}"
IMAGE_PUSHED=true

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
