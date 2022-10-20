# SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR            := $(TOOLS_DIR)/bin
KUBEBUILDER_K8S_VERSION  := 1.24.2
KUBEBUILDER_TAG          := $(TOOLS_BIN_DIR)/kubebuilder
KUBEBUILDER_DIR          := $(TOOLS_BIN_DIR)/kubebuilder_$(KUBEBUILDER_K8S_VERSION)
KUBEBUILDER_ASSETS       := "$(shell realpath $(KUBEBUILDER_DIR))/bin"

# Use this "function" to add the version file as a prerequisite for the tool target: e.g.
#   $(HELM): $(call tool_version_file,$(HELM),$(HELM_VERSION))
tool_version_file = $(TOOLS_BIN_DIR)/.version_$(subst $(TOOLS_BIN_DIR)/,,$(1))_$(2)

# This target cleans up any previous version files for the given tool and creates the given version file.
# This way, we can generically determine, which version was installed without calling each and every binary explicitly.
$(TOOLS_BIN_DIR)/.version_%:
	@mkdir -p $(TOOLS_BIN_DIR)
	@version_file=$@; rm -f $${version_file%_*}*
	@touch $@

$(KUBEBUILDER_DIR): $(call tool_version_file,$(KUBEBUILDER_TAG),$(KUBEBUILDER_K8S_VERSION))
	curl -sSL https://go.kubebuilder.io/test-tools/$(KUBEBUILDER_K8S_VERSION)/$(shell go env GOOS)/$(shell go env GOARCH) | tar -xvz
	@mv kubebuilder $(KUBEBUILDER_DIR)
	@touch $(KUBEBUILDER_TAG)
