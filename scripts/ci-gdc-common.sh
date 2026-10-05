#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

# check_ci_preconditions verifies whether the integration test should run when
# triggered by GitHub Actions, and checks out the target PR on manual dispatch.
check_ci_preconditions() {
  if [[ "${GITHUB_ACTIONS:-}" != "true" ]]; then
    return 0
  fi

  if [[ -z "${CI_LOG_FILE:-}" ]]; then
    CI_LOG_FILE="$(mktemp /tmp/ci-integration-XXXXXX.log)"
    export CI_LOG_FILE
    exec > >(tee -a "${CI_LOG_FILE}") 2>&1
    CI_TEE_PID=$!
    export CI_TEE_PID
  fi

  # For pull_request events, only run automatically on same-repository PRs
  # authored by a non-bot repository collaborator or Gardener organization member.
  # Exiting with code 1 when skipped ensures the required status check blocks
  # merging until a maintainer manually triggers workflow_dispatch with pr_number.
  if [[ "${GITHUB_EVENT_NAME:-}" == "pull_request" && -f "${GITHUB_EVENT_PATH:-}" ]]; then
    local head_repo author_association pr_user pr_number
    head_repo=$(jq -r '.pull_request.head.repo.full_name // empty' "${GITHUB_EVENT_PATH}")
    author_association=$(jq -r '.pull_request.author_association // empty' "${GITHUB_EVENT_PATH}")
    pr_user=$(jq -r '.pull_request.user.login // empty' "${GITHUB_EVENT_PATH}")
    pr_number=$(jq -r '.pull_request.number // empty' "${GITHUB_EVENT_PATH}")

    if [[ "${GITHUB_ACTOR:-}" == *"[bot]" || "${GITHUB_ACTOR:-}" == *"-robot" || "${pr_user}" == *"[bot]" || "${pr_user}" == *"-robot" ]]; then
      echo "Skipping automatic integration test for automated bot account (${pr_user:-${GITHUB_ACTOR}})."
      echo "A maintainer must trigger this test manually via workflow_dispatch with pr_number=${pr_number}."
      exit 1
    fi

    if [[ "${head_repo}" != "${GITHUB_REPOSITORY:-}" ]]; then
      echo "Skipping automatic integration test for forked PR (${head_repo})."
      echo "A maintainer must trigger this test manually via workflow_dispatch with pr_number=${pr_number}."
      if ! gh api "repos/${GITHUB_REPOSITORY}/contents/.github/workflows/gdc-integration-test.yaml" >/dev/null 2>&1; then
        echo "Workflow .github/workflows/gdc-integration-test.yaml is not yet on the default branch of ${GITHUB_REPOSITORY}; skipping until merged."
        exit 0
      fi
      exit 1
    fi

    case "${author_association}" in
      COLLABORATOR|MEMBER|OWNER)
        ;;
      *)
        echo "Skipping automatic integration test for untrusted author_association '${author_association}'."
        echo "A maintainer must trigger this test manually via workflow_dispatch with pr_number=${pr_number}."
        exit 1
        ;;
    esac
  fi

  # When triggered manually via workflow_dispatch with a pr_number input,
  # fetch and check out that PR's head commit and attach a check-run to its SHA.
  if [[ "${GITHUB_EVENT_NAME:-}" == "workflow_dispatch" && -n "${PR_NUMBER:-}" ]]; then
    echo "Fetching and checking out PR #${PR_NUMBER}..."
    git fetch origin "pull/${PR_NUMBER}/head:pr-${PR_NUMBER}"
    git checkout "pr-${PR_NUMBER}"
    PR_HEAD_SHA="$(git rev-parse HEAD)"
    export PR_HEAD_SHA
    start_pr_check_run "${CHECK_NAME:-GDC External DNS Management Integration Test (GDC Staging)}"
  fi
}

# start_pr_check_run creates an in_progress GitHub Check Run on PR_HEAD_SHA when
# triggered via workflow_dispatch so the manual run satisfies required status checks on the PR.
start_pr_check_run() {
  local check_name="$1"
  if [[ -z "${PR_HEAD_SHA:-}" || -z "${GH_TOKEN:-}" || -z "${GITHUB_REPOSITORY:-}" ]]; then
    return 0
  fi

  local run_url="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID:-}"
  echo "Creating in_progress check-run '${check_name}' on PR #${PR_NUMBER} commit ${PR_HEAD_SHA}..."
  PR_CHECK_RUN_ID=$(gh api --method POST "repos/${GITHUB_REPOSITORY}/check-runs" \
    -f name="${check_name}" \
    -f head_sha="${PR_HEAD_SHA}" \
    -f status="in_progress" \
    -f details_url="${run_url}" \
    -f "output[title]=Running via manual workflow_dispatch" \
    -f "output[summary]=Triggered manually by @${GITHUB_ACTOR:-maintainer} for PR #${PR_NUMBER} (${run_url})." \
    --jq '.id' 2>/dev/null || true)
  export PR_CHECK_RUN_ID
}

# complete_pr_check_run emits a GitHub Actions failure annotation (if non-zero)
# and updates the manual check-run on PR_HEAD_SHA with the final conclusion.
complete_pr_check_run() {
  local exit_code="$1"
  if [[ -n "${PR_CHECK_RUN_ID:-}" && -n "${GH_TOKEN:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
    local conclusion="failure"
    local title="Integration test failed"
    if [[ "${exit_code}" -eq 0 ]]; then
      conclusion="success"
      title="Integration test passed"
    fi

    local run_url="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID:-}"
    echo "Updating check-run ${PR_CHECK_RUN_ID} on PR #${PR_NUMBER} (${PR_HEAD_SHA}) with conclusion=${conclusion}..."
    gh api --method PATCH "repos/${GITHUB_REPOSITORY}/check-runs/${PR_CHECK_RUN_ID}" \
      -f status="completed" \
      -f conclusion="${conclusion}" \
      -f details_url="${run_url}" \
      -f "output[title]=${title}" \
      -f "output[summary]=Manual workflow_dispatch run completed with ${conclusion} (${run_url})." >/dev/null 2>&1 || \
      echo "Warning: Failed to update check-run ${PR_CHECK_RUN_ID} on PR #${PR_NUMBER}."
  fi

  if [[ "${GITHUB_ACTIONS:-}" == "true" && -n "${CI_LOG_FILE:-}" ]]; then
    sleep 1
    if [[ "${exit_code}" -ne 0 && -s "${CI_LOG_FILE}" ]]; then
      local err_lines tail_log combined_log
      err_lines=$(grep -v '^::add-mask::' "${CI_LOG_FILE}" | grep -E '(--- FAIL:|FAIL[[:space:]]|Error:|fatalf|Fatalf|panic:|timed out|Back-off|ErrImagePull)' | tail -n 20 || true)
      tail_log=$(grep -v '^::add-mask::' "${CI_LOG_FILE}" | tail -n 35 || true)
      if [[ -n "${err_lines}" ]]; then
        combined_log=$(printf '=== Failure Summary ===\n%s\n=== Log Tail ===\n%s' "${err_lines}" "${tail_log}" | sed ':a;N;$!ba;s/%/%25/g;s/\r/%0D/g;s/\n/%0A/g')
      else
        combined_log=$(printf '%s' "${tail_log}" | sed ':a;N;$!ba;s/%/%25/g;s/\r/%0D/g;s/\n/%0A/g')
      fi
      echo "::error title=${CHECK_NAME:-Integration Test} Failed::${combined_log}"
    fi
    sleep 1
    rm -f "${CI_LOG_FILE}"
    exec >/dev/null 2>&1 || true
    if [[ -n "${CI_TEE_PID:-}" ]]; then
      pkill -P "${CI_TEE_PID}" 2>/dev/null || true
      kill "${CI_TEE_PID}" 2>/dev/null || true
    fi
  fi
}

# setup_gdc_credentials writes the GDC service account key to disk (if provided
# via environment variable), masks all lines in GitHub Actions logs, and
# downloads the GDC Root CA certificate.
setup_gdc_credentials() {
  # Ensure xtrace is disabled while handling the secret key.
  local xtrace_was_set=false
  if [[ "$-" == *x* ]]; then
    xtrace_was_set=true
    set +x
  fi

  local sa_file="$1"
  local ca_file="$2"
  local org="$3"
  local zone="$4"
  local lab_url="$5"
  local sa_key="${6:-}"

  if [[ -n "${sa_key}" ]]; then
    # Mask every non-empty line of the raw secret in GitHub Actions logs.
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      while IFS= read -r line; do
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"
        if [[ -n "${line}" && "${line}" != "{" && "${line}" != "}" ]]; then
          echo "::add-mask::${line}"
        fi
      done <<< "${sa_key}"
    fi

    # Normalize the JSON payload in case terminal line-wrapping or trailing prompt
    # characters were introduced when copying the secret into GitHub Environment secrets.
    local normalized_sa=""
    if normalized_sa=$(printf '%s' "${sa_key}" | jq . 2>/dev/null) && [[ -n "${normalized_sa}" ]]; then
      sa_key="${normalized_sa}"
    elif normalized_sa=$(printf '%s' "${sa_key}" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//' | tr -d '\r\n' | sed 's/^[^{]*//; s/[^}]*$//; s/BEGINECPRIVATEKEY/BEGIN EC PRIVATE KEY/g; s/ENDECPRIVATEKEY/END EC PRIVATE KEY/g; s/BEGINPRIVATEKEY/BEGIN PRIVATE KEY/g; s/ENDPRIVATEKEY/END PRIVATE KEY/g' | jq . 2>/dev/null) && [[ -n "${normalized_sa}" ]]; then
      sa_key="${normalized_sa}"
    fi

    # Also mask every non-empty line of the normalized multi-line JSON key.
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      while IFS= read -r line; do
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"
        if [[ -n "${line}" && "${line}" != "{" && "${line}" != "}" ]]; then
          echo "::add-mask::${line}"
        fi
      done <<< "${sa_key}"
    fi

    (
      umask 077
      printf '%s\n' "${sa_key}" > "${sa_file}"
    )
  fi

  if [[ "${xtrace_was_set}" == "true" ]]; then
    set -x
  fi

  if [[ ! -s "${sa_file}" ]]; then
    echo "::error::GDC Service Account key is required (file '${sa_file}' is missing or empty). Verify that GDC_DNS_SERVICE_ACCOUNT_KEY is configured in the gdc-staging environment secrets."
    exit 1
  fi

  echo "Fetching GDC Root CA from console.${org}.${zone}.${lab_url}..."
  if ! wget "https://console.${org}.${zone}.${lab_url}/.well-known/certificate-authority" \
    --no-check-certificate -q -O "${ca_file}"; then
    echo "::error::Failed to fetch GDC Root CA from console.${org}.${zone}.${lab_url}"
    exit 1
  fi
}

# install_gdcloud_cli downloads and installs the gdcloud CLI and
# gdcloud-k8s-auth-plugin from the GDC management API server's CLIBundleMetadata.
install_gdcloud_cli() {
  local sa_file="$1"
  local ca_file="$2"
  local mgmt_url="$3"
  local gdcloud_version="$4"
  local token_helper_pkg="${5:-./test/integration/gdc/cmd/token-helper}"

  local gdcloud_root="/usr/local/bin/google-distributed-cloud-hosted-cli"
  if [[ -x "${gdcloud_root}/bin/gdcloud" && -x "${gdcloud_root}/bin/gdcloud-k8s-auth-plugin" ]]; then
    echo "gdcloud CLI already installed at ${gdcloud_root}/bin/gdcloud"
    export PATH="${gdcloud_root}/bin:${PATH}"
    export GDCLOUD_PATH="${gdcloud_root}/bin/gdcloud"
    return 0
  fi

  echo "Minting STS token using GDC ServiceAccount to query CLIBundleMetadata..."
  local sts_token
  if ! sts_token=$(go run "${token_helper_pkg}" \
    --service-account-file="${sa_file}" \
    --ca-cert-file="${ca_file}" \
    --audience="${mgmt_url}"); then
    echo "::error::Failed to mint STS token using token-helper against ${mgmt_url}"
    exit 1
  fi

  echo "Resolving gdcloud CLI bundle URL (version: ${gdcloud_version}) from ${mgmt_url}..."
  local serving_url
  serving_url=$(curl -ksSL -H "Authorization: Bearer ${sts_token}" \
    "${mgmt_url}/apis/artifactview.private.gdc.goog/v1alpha1/namespaces/ui-system/clibundlemetadata" | \
    jq -r --arg ver "${gdcloud_version}" '
      [.items[]
       | select(.platform.os == "linux" and .platform.architecture == "amd64")
       | select($ver == "" or (.commonMetadata.artifactVersion | contains($ver)))
      ]
      | sort_by(.metadata.creationTimestamp)
       | last
       | .commonMetadata.servingURL // empty
    ')

  if [[ -z "${serving_url}" ]]; then
    echo "::error::Failed to resolve gdcloud CLI servingURL for version '${gdcloud_version}'."
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

# delete_ghcr_image_tag removes the temporary PR container image version from
# ghcr.io via the GitHub Packages REST API after the test finishes.
delete_ghcr_image_tag() {
  local image_repo="$1"
  local image_tag="$2"

  if [[ "${GITHUB_ACTIONS:-}" != "true" || -z "${GH_TOKEN:-}" ]]; then
    return 0
  fi

  local owner="${GITHUB_REPOSITORY_OWNER:-}"
  local pkg_name="${image_repo#ghcr.io/${owner}/}"
  local encoded_pkg="${pkg_name//\//%2F}"

  echo "Cleaning up temporary container image ${image_repo}:${image_tag} from ghcr.io..."
  local scope
  for scope in "orgs/${owner}" "users/${owner}"; do
    local version_id
    version_id=$(gh api "/${scope}/packages/container/${encoded_pkg}/versions" \
      --jq ".[] | select(.metadata.container.tags[]? == \"${image_tag}\") | .id" 2>/dev/null | head -n 1 || true)

    if [[ -n "${version_id}" ]]; then
      # Delete the specific package version; if it is the sole version in the package,
      # fall back to deleting the temporary package itself.
      if ! gh api --method DELETE "/${scope}/packages/container/${encoded_pkg}/versions/${version_id}" >/dev/null 2>&1; then
          echo "Warning: Failed to delete ${image_repo}:${image_tag} from ghcr.io."
      fi
      return 0
    fi
  done
}

# retry_cmd retries a command up to 3 times with backoff on transient failures.
retry_cmd() {
  local attempt
  for attempt in 1 2 3; do
    if "$@"; then
      return 0
    fi
    if [[ "${attempt}" -lt 3 ]]; then
      echo "Command '$*' failed on attempt ${attempt}; retrying in $((attempt * 5))s..."
      sleep $((attempt * 5))
    fi
  done
  return 1
}
