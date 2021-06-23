#!/bin/bash
#
# Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# SPDX-License-Identifier: Apache-2.0

set -e

SOURCE_PATH="$(dirname $0)/.."
IMAGE_REGISTRY="$(${SOURCE_PATH}/hack/get-image-registry.sh)"
CD_REGISTRY="$(${SOURCE_PATH}/hack/get-cd-registry.sh)"
COMPONENT_NAME="$(${SOURCE_PATH}/hack/get-cd-component-name.sh)"

CA_PATH="$(mktemp -d)"
TMP_COMPONENT_DESCRIPTOR_PATH="${CA_PATH}/component-descriptor.yaml"

if ! which component-cli 1>/dev/null; then
  echo -n "component-cli is required to generate the component descriptors"
  echo -n "Trying to installing it..."
  go get github.com/gardener/component-cli/cmd/component-cli

  if ! which component-cli 1>/dev/null; then
    echo -n "component-cli was successfully installed but the binary cannot be found"
    echo -n "Try adding the \$GOPATH/bin to your \$PATH..."
    exit 1
  fi
fi
if ! which jq 1>/dev/null; then
  echo -n "jq canot be found"
  exit 1
fi

echo "> Generate Component Descriptor ${EFFECTIVE_VERSION}"
echo "> Creating base definition"
component-cli ca create "${CA_PATH}" \
    --component-name=${COMPONENT_NAME} \
    --component-version=${EFFECTIVE_VERSION} \
    --repo-ctx=${CD_REGISTRY}

echo "> Extending resources.yaml: adding image of virtual-garden deployer"
RESOURCES_BASE_PATH="$(mktemp -d)"
RESOURCES_FILE_PATH="${RESOURCES_BASE_PATH}/resources.yaml"
cat << EOF > ${RESOURCES_FILE_PATH}
---
type: ociImage
name: dns-controller-manager
relation: local
access:
  type: ociRegistry
  imageReference: ${IMAGE_REGISTRY}/dns-controller-manager:${EFFECTIVE_VERSION}
...
EOF

echo "> Adding image resources to ${CA_PATH}"
component-cli component-archive resources add "${CA_PATH}" "${RESOURCES_FILE_PATH}"

SOURCE_PATH="$(dirname $0)/.."
echo "> Creating ctf folder"

CTF_DIR="$(mktemp -d)"
CTF_PATH="${CTF_DIR}/ctf.tar"

COMPONENT_DESCRIPTOR_DIR="$(mktemp -d)"
COMPONENT_DESCRIPTOR_FILE_PATH="${COMPONENT_DESCRIPTOR_DIR}/component_descriptor.yaml"

ADD_DEPENDENCIES_CMD="echo"

CTF_PATH=${CTF_PATH} BASE_DEFINITION_PATH=${TMP_COMPONENT_DESCRIPTOR_PATH} \
  COMPONENT_DESCRIPTOR_PATH=${COMPONENT_DESCRIPTOR_FILE_PATH} \
  ADD_DEPENDENCIES_CMD=${ADD_DEPENDENCIES_CMD} bash $SOURCE_PATH/.ci/component_descriptor

echo "> Uploading archive from ${CTF_PATH}"
component-cli ctf push --repo-ctx=${CD_REGISTRY} "${CTF_PATH}"

