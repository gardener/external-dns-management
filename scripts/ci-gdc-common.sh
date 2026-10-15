#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

# is_authorized_maintainer checks whether a GitHub user is an authorized
# maintainer listed in OWNERS_ALIASES, OWNERS, or CODEOWNERS read strictly
# from origin/master.
is_authorized_maintainer() {
  local actor="$1"

  if [[ -z "${actor}" ]]; then
    return 1
  fi

  if [[ "${actor}" == *"[bot]" || "${actor}" == *"-robot" ]]; then
    return 1
  fi

  local actor_lc owners_list="" owners_ref="HEAD"
  actor_lc=$(printf '%s' "${actor}" | tr '[:upper:]' '[:lower:]')
  if git rev-parse --verify origin/master >/dev/null 2>&1; then
    owners_ref="origin/master"
  fi

  local owners_aliases_content owners_content codeowners_content
  owners_aliases_content=$(git show "${owners_ref}:OWNERS_ALIASES" 2>/dev/null || true)
  if [[ -n "${owners_aliases_content}" ]]; then
    owners_list+=$(printf '%s\n' "${owners_aliases_content}" | grep -E '^[[:space:]]*-[[:space:]]*[A-Za-z0-9_-]+' | sed -E 's/^[[:space:]]*-[[:space:]]*//; s/[[:space:]]*$//' || true)
    owners_list+=$'\n'
  fi

  owners_content=$(git show "${owners_ref}:OWNERS" 2>/dev/null || true)
  if [[ -n "${owners_content}" ]]; then
    owners_list+=$(printf '%s\n' "${owners_content}" | grep -E '^[[:space:]]*-[[:space:]]*[A-Za-z0-9_-]+' | sed -E 's/^[[:space:]]*-[[:space:]]*//; s/[[:space:]]*$//' || true)
    owners_list+=$'\n'
  fi

  codeowners_content=$(git show "${owners_ref}:CODEOWNERS" 2>/dev/null || true)
  if [[ -n "${codeowners_content}" ]]; then
    owners_list+=$(printf '%s\n' "${codeowners_content}" | grep -v '^#' | grep -oE '@[A-Za-z0-9_-]+' | tr -d '@' || true)
  fi

  if printf '%s\n' "${owners_list}" | tr '[:upper:]' '[:lower:]' | grep -Fxq "${actor_lc}"; then
    return 0
  fi

  return 1
}

# prepare_trusted_ci_files copies the CI runner scripts and compiles the
# token-helper binary into an isolated directory (/tmp/ci-trusted) outside the
# repository working tree before any untrusted PR ref is checked out. When
# origin/master already contains the updated CI scripts and token-helper, they
# are extracted and built strictly from origin/master.
prepare_trusted_ci_files() {
  CI_TRUSTED_DIR="/tmp/ci-trusted"
  rm -rf "${CI_TRUSTED_DIR}"
  mkdir -p "${CI_TRUSTED_DIR}/scripts" "${CI_TRUSTED_DIR}/bin"

  if git rev-parse --verify origin/master >/dev/null 2>&1 && \
     git show origin/master:scripts/ci-gdc-common.sh 2>/dev/null | grep -q 'verify_pr_head_at_comment_time' && \
     git cat-file -e origin/master:test/integration/gdc/cmd/token-helper/main.go 2>/dev/null; then
    echo "Staging trusted CI scripts and token-helper from origin/master..."
    git show origin/master:scripts/ci-gdc-common.sh > "${CI_TRUSTED_DIR}/scripts/ci-gdc-common.sh"
    git show origin/master:scripts/ci-gdc-integration-test.sh > "${CI_TRUSTED_DIR}/scripts/ci-gdc-integration-test.sh"
    mkdir -p "${CI_TRUSTED_DIR}/src"
    git archive origin/master | tar -x -C "${CI_TRUSTED_DIR}/src"
    (
      cd "${CI_TRUSTED_DIR}/src"
      go build -o "${CI_TRUSTED_DIR}/bin/token-helper" ./test/integration/gdc/cmd/token-helper
    )
    rm -rf "${CI_TRUSTED_DIR}/src"
  else
    echo "Staging trusted CI scripts and token-helper from current base checkout..."
    cp ./scripts/ci-gdc-common.sh "${CI_TRUSTED_DIR}/scripts/ci-gdc-common.sh"
    cp ./scripts/ci-gdc-integration-test.sh "${CI_TRUSTED_DIR}/scripts/ci-gdc-integration-test.sh"
    if [[ -f "./test/integration/gdc/cmd/token-helper/main.go" ]]; then
      go build -o "${CI_TRUSTED_DIR}/bin/token-helper" ./test/integration/gdc/cmd/token-helper
    fi
  fi

  chmod +x "${CI_TRUSTED_DIR}/scripts/ci-gdc-common.sh" "${CI_TRUSTED_DIR}/scripts/ci-gdc-integration-test.sh"
  export CI_TRUSTED_DIR
}

# verify_pr_head_at_comment_time ensures that when triggered via
# '/test-gdc-integration' (issue_comment), the fetched PR head commit already
# existed on the PR before the comment was posted, preventing a TOCTOU race if a
# commit is pushed right after the maintainer comments.
verify_pr_head_at_comment_time() {
  if [[ "${GITHUB_EVENT_NAME:-}" != "issue_comment" || ! -f "${GITHUB_EVENT_PATH:-}" ]]; then
    return 0
  fi

  local comment_created_at comment_epoch
  comment_created_at=$(jq -r '.comment.created_at // empty' "${GITHUB_EVENT_PATH}")
  if [[ -z "${comment_created_at}" ]]; then
    echo "::error::Missing .comment.created_at in issue_comment event payload."
    exit 1
  fi
  comment_epoch=$(date -d "${comment_created_at}" +%s)

  local commit_date commit_epoch
  commit_date=$(git show -s --format=%cI "${PR_HEAD_SHA}")
  commit_epoch=$(date -d "${commit_date}" +%s)
  if [[ "${commit_epoch}" -gt "${comment_epoch}" ]]; then
    echo "::error::PR #${PR_NUMBER} head commit ${PR_HEAD_SHA:0:7} was committed at ${commit_date}, which is after the '/test-gdc-integration' comment was posted at ${comment_created_at}. Aborting to prevent executing unreviewed code."
    exit 1
  fi

  if [[ -n "${GH_TOKEN:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
    local latest_timeline_event_at
    latest_timeline_event_at=$(gh api --paginate "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/timeline?per_page=100" 2>/dev/null | \
      jq -rs '[ .[][] | select(.event == "committed" or .event == "head_ref_force_pushed") | (.created_at // .committer.date // empty) | select(length > 0) ] | sort | last // empty' || true)
    if [[ -n "${latest_timeline_event_at}" ]]; then
      local timeline_epoch
      timeline_epoch=$(date -d "${latest_timeline_event_at}" +%s)
      if [[ "${timeline_epoch}" -gt "${comment_epoch}" ]]; then
        echo "::error::PR #${PR_NUMBER} had a commit or force-push at ${latest_timeline_event_at}, which is after the '/test-gdc-integration' comment was posted at ${comment_created_at}. Aborting to prevent executing unreviewed code."
        exit 1
      fi
    fi

    local earliest_suite_at
    earliest_suite_at=$(gh api "repos/${GITHUB_REPOSITORY}/commits/${PR_HEAD_SHA}/check-suites" \
      --jq '[ .check_suites[]?.created_at // empty | select(length > 0) ] | sort | first // empty' 2>/dev/null || true)
    if [[ -n "${earliest_suite_at}" ]]; then
      local suite_epoch
      suite_epoch=$(date -d "${earliest_suite_at}" +%s)
      if [[ "${suite_epoch}" -gt "${comment_epoch}" ]]; then
        echo "::error::PR #${PR_NUMBER} head commit ${PR_HEAD_SHA:0:7} was pushed to GitHub at ${earliest_suite_at}, which is after the '/test-gdc-integration' comment was posted at ${comment_created_at}. Aborting to prevent executing unreviewed code."
        exit 1
      fi
    fi
  fi

  echo "Verified PR #${PR_NUMBER} head commit ${PR_HEAD_SHA:0:7} predates '/test-gdc-integration' comment (${comment_created_at})."
}

# should_skip_non_code_changes returns 0 (true) on pull_request / pull_request_target
# events when all files modified in the PR relative to origin/master are CI scripts,
# workflow configs, or documentation (excluding Go provider/test code and examples).
should_skip_non_code_changes() {
  if [[ "${GITHUB_EVENT_NAME:-}" != "pull_request" && "${GITHUB_EVENT_NAME:-}" != "pull_request_target" ]]; then
    return 1
  fi

  local non_code_regex='^(\.gitignore|\.dockerignore|\.golangci\.yaml|OWNERS.*|CODEOWNERS|LICENSE.*|NOTICE.*|VERSION|Makefile|.*\.md|docs/.*|\.github/.*|scripts/ci-gdc-.*\.sh)$'
  local pr_head_sha="${PR_HEAD_SHA:-}"
  if [[ -z "${pr_head_sha}" && -f "${GITHUB_EVENT_PATH:-}" ]]; then
    pr_head_sha=$(jq -r '.pull_request.head.sha // empty' "${GITHUB_EVENT_PATH}")
  fi
  if [[ -z "${pr_head_sha}" ]]; then
    pr_head_sha=$(git rev-parse HEAD)
  fi

  if ! git rev-parse --verify origin/master >/dev/null 2>&1; then
    git fetch origin master >/dev/null 2>&1 || true
  fi

  local pr_files
  pr_files=$(git diff --name-only "origin/master...${pr_head_sha}" 2>/dev/null || git diff --name-only "origin/master" "${pr_head_sha}" 2>/dev/null || true)
  if [[ -n "${pr_files}" ]]; then
    if ! printf '%s\n' "${pr_files}" | grep -qvE "${non_code_regex}"; then
      echo "All files changed in PR (${pr_files//$'\n'/, }) are CI/non-code files. Skipping automatic GDC cluster integration test."
      return 0
    fi
  fi

  return 1
}

# check_ci_preconditions verifies whether the integration test should run when
# triggered by GitHub Actions, and checks out the target PR on pull_request_target,
# '/test-gdc-integration' comment, or manual workflow_dispatch.
check_ci_preconditions() {
  if [[ "${GITHUB_ACTIONS:-}" != "true" ]]; then
    return 0
  fi

  if [[ "${CI_STAGE_PRECONDITIONS:-}" != "true" && -z "${CI_LOG_FILE:-}" ]]; then
    CI_LOG_FILE="$(mktemp /tmp/ci-integration-XXXXXX.log)"
    export CI_LOG_FILE
    exec > >(tee -a "${CI_LOG_FILE}") 2>&1
    CI_TEE_PID=$!
    export CI_TEE_PID
  fi

  if [[ "${CI_PRECONDITIONS_VERIFIED:-}" == "true" ]]; then
    return 0
  fi

  # For pull_request and pull_request_target events, verify the PR author is an
  # authorized maintainer on origin/master before checking out and running PR code.
  if [[ ( "${GITHUB_EVENT_NAME:-}" == "pull_request" || "${GITHUB_EVENT_NAME:-}" == "pull_request_target" ) && -f "${GITHUB_EVENT_PATH:-}" ]]; then
    local author_association pr_user pr_number
    author_association=$(jq -r '.pull_request.author_association // empty' "${GITHUB_EVENT_PATH}")
    pr_user=$(jq -r '.pull_request.user.login // empty' "${GITHUB_EVENT_PATH}")
    pr_number=$(jq -r '.pull_request.number // empty' "${GITHUB_EVENT_PATH}")
    PR_NUMBER="${pr_number}"
    export PR_NUMBER

    if [[ "${GITHUB_ACTOR:-}" == *"[bot]" || "${GITHUB_ACTOR:-}" == *"-robot" || "${pr_user}" == *"[bot]" || "${pr_user}" == *"-robot" ]]; then
      echo "Skipping automatic integration test for automated bot account (${pr_user:-${GITHUB_ACTOR}})."
      echo "A maintainer must trigger this test manually by commenting '/test-gdc-integration' on the PR or via workflow_dispatch with pr_number=${pr_number}."
      exit 1
    fi

    if ! is_authorized_maintainer "${pr_user}"; then
      echo "Skipping automatic integration test for untrusted author '${pr_user}' (author_association='${author_association}')."
      echo "A maintainer must trigger this test manually by commenting '/test-gdc-integration' on the PR or via workflow_dispatch with pr_number=${pr_number}."
      exit 1
    fi

    if should_skip_non_code_changes; then
      if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "should_run=false" >> "${GITHUB_OUTPUT}"
      fi
      exit 0
    fi
  fi

  if [[ "${GITHUB_EVENT_NAME:-}" == "issue_comment" && -f "${GITHUB_EVENT_PATH:-}" ]]; then
    local is_pr comment_body comment_user comment_assoc comment_id cmd_line
    is_pr=$(jq -r '.issue.pull_request.url // empty' "${GITHUB_EVENT_PATH}")
    comment_body=$(jq -r '.comment.body // empty' "${GITHUB_EVENT_PATH}")
    comment_user=$(jq -r '.comment.user.login // empty' "${GITHUB_EVENT_PATH}")
    comment_assoc=$(jq -r '.comment.author_association // empty' "${GITHUB_EVENT_PATH}")
    comment_id=$(jq -r '.comment.id // empty' "${GITHUB_EVENT_PATH}")
    PR_NUMBER=$(jq -r '.issue.number // empty' "${GITHUB_EVENT_PATH}")
    export PR_NUMBER

    cmd_line=$(printf '%s\n' "${comment_body}" | tr -d '\r' | grep -E '^[[:space:]]*/test-gdc-integration([[:space:]]|$)' | head -n 1 || true)
    if [[ -z "${is_pr}" || -z "${cmd_line}" ]]; then
      echo "Comment does not contain a '/test-gdc-integration' command line on a pull request. Skipping."
      if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "should_run=false" >> "${GITHUB_OUTPUT}"
      fi
      exit 0
    fi

    if ! is_authorized_maintainer "${comment_user}"; then
      echo "::error::User '${comment_user}' (author_association='${comment_assoc}') is not listed as an authorized maintainer on origin/master and cannot trigger '/test-gdc-integration'."
      exit 1
    fi

    if [[ -n "${comment_id}" && -n "${GH_TOKEN:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
      gh api --method POST "repos/${GITHUB_REPOSITORY}/issues/comments/${comment_id}/reactions" \
        -f content='rocket' >/dev/null 2>&1 || true
    fi
  fi

  if [[ "${GITHUB_EVENT_NAME:-}" == "workflow_dispatch" ]]; then
    if ! is_authorized_maintainer "${GITHUB_ACTOR:-}"; then
      echo "::error::User '${GITHUB_ACTOR:-}' is not listed as an authorized maintainer on origin/master and cannot trigger workflow_dispatch."
      exit 1
    fi
  fi

  # Stage trusted CI scripts and compile token-helper before checking out any PR branch.
  prepare_trusted_ci_files

  # When triggered via pull_request_target, '/test-gdc-integration' comment, or
  # workflow_dispatch with a pr_number input, fetch and check out that PR's head
  # commit, merge origin/master, and attach a check-run to the PR's head SHA.
  if [[ ( "${GITHUB_EVENT_NAME:-}" == "pull_request_target" || "${GITHUB_EVENT_NAME:-}" == "workflow_dispatch" || "${GITHUB_EVENT_NAME:-}" == "issue_comment" ) && -n "${PR_NUMBER:-}" ]]; then
    local base_branch_sha
    if git rev-parse --verify origin/master >/dev/null 2>&1; then
      base_branch_sha="$(git rev-parse origin/master)"
    else
      base_branch_sha="$(git rev-parse HEAD)"
    fi
    echo "Fetching and checking out PR #${PR_NUMBER}..."
    git fetch origin "pull/${PR_NUMBER}/head:pr-${PR_NUMBER}"
    PR_HEAD_SHA="$(git rev-parse "pr-${PR_NUMBER}")"
    export PR_HEAD_SHA
    COMMIT_HASH="${PR_HEAD_SHA:0:7}"
    export COMMIT_HASH
    verify_pr_head_at_comment_time
    if [[ "${GITHUB_EVENT_NAME:-}" == "workflow_dispatch" || "${GITHUB_EVENT_NAME:-}" == "issue_comment" ]]; then
      start_pr_check_run "${CHECK_NAME:-GDC External DNS Management Integration Test (GDC Staging)}"
    fi
    git checkout "pr-${PR_NUMBER}"
    echo "Merging base branch (${base_branch_sha:0:7}) into PR #${PR_NUMBER} (${COMMIT_HASH})..."
    if ! git -c user.name="github-actions[bot]" -c user.email="github-actions[bot]@users.noreply.github.com" \
      merge --no-edit "${base_branch_sha}"; then
      echo "::error::Failed to merge base branch (${base_branch_sha:0:7}) into PR #${PR_NUMBER} (${COMMIT_HASH}). Please rebase the PR onto master."
      complete_pr_check_run 1
      exit 1
    fi
  fi
}

# verify_and_prepare_ci runs in the dedicated secret-less precondition step in
# GitHub Actions and exports verified state to GITHUB_ENV and GITHUB_OUTPUT.
verify_and_prepare_ci() {
  CI_STAGE_PRECONDITIONS=true check_ci_preconditions

  if [[ -n "${GITHUB_ENV:-}" ]]; then
    {
      echo "CI_PRECONDITIONS_VERIFIED=true"
      echo "CI_TRUSTED_DIR=${CI_TRUSTED_DIR}"
      [[ -n "${PR_NUMBER:-}" ]] && echo "PR_NUMBER=${PR_NUMBER}"
      [[ -n "${PR_HEAD_SHA:-}" ]] && echo "PR_HEAD_SHA=${PR_HEAD_SHA}"
      [[ -n "${PR_CHECK_RUN_ID:-}" ]] && echo "PR_CHECK_RUN_ID=${PR_CHECK_RUN_ID}"
      [[ -n "${COMMIT_HASH:-}" ]] && echo "COMMIT_HASH=${COMMIT_HASH}"
    } >> "${GITHUB_ENV}"
  fi

  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "should_run=true" >> "${GITHUB_OUTPUT}"
  fi
}

# start_pr_check_run creates an in_progress GitHub Check Run on PR_HEAD_SHA when
# triggered via '/test-gdc-integration' or workflow_dispatch so the manual run
# reports status on the PR commit.
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
    -f "output[title]=Running GDC integration test" \
    -f "output[summary]=Triggered via ${GITHUB_EVENT_NAME:-workflow} by @${GITHUB_ACTOR:-maintainer} for PR #${PR_NUMBER} (${run_url})." \
    --jq '.id' 2>/dev/null || true)
  export PR_CHECK_RUN_ID
}

# complete_pr_check_run emits a GitHub Actions failure annotation (if non-zero)
# and updates the check-run on PR_HEAD_SHA with the final conclusion.
complete_pr_check_run() {
  local exit_code="$1"
  if [[ -n "${PR_CHECK_RUN_ID:-}" && -n "${GH_TOKEN:-}" && -z "${PR_CHECK_RUN_COMPLETED:-}" && -n "${GITHUB_REPOSITORY:-}" ]]; then
    PR_CHECK_RUN_COMPLETED=true
    export PR_CHECK_RUN_COMPLETED
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
      -f "output[summary]=GDC integration test run completed with ${conclusion} (${run_url})." >/dev/null 2>&1 || \
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
  local -a token_helper_cmd=("go" "run" "${token_helper_pkg}")
  if [[ -n "${CI_TRUSTED_DIR:-}" && -x "${CI_TRUSTED_DIR}/bin/token-helper" ]]; then
    token_helper_cmd=("${CI_TRUSTED_DIR}/bin/token-helper")
  elif [[ -x "${token_helper_pkg}" ]]; then
    token_helper_cmd=("${token_helper_pkg}")
  fi
  if ! sts_token=$("${token_helper_cmd[@]}" \
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
