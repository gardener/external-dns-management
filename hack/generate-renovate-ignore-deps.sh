#!/bin/bash
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


# Takes the content of a go.mod file and an array to add the extracted dependencies to.
extract_dependencies() {
    local go_mod=$1
    local dependencies=$2

    while IFS= read -r line; do
        dependency=$(echo "$line" | awk '{print $1}') # Splits the line by spaces and takes the first part omitting the version and the //indirect comment.
        eval "$dependencies+=('$dependency')"
    done <<< "$go_mod"
}

echo "🪧 Generating ignoreDeps section for 'renovate.json5'"
echo "🛜 Downloading the latest 'go.mod' from gardener/gardener..."

# Only the dependencies in a `go.mod` file are indented with a tab.
local_go_mod=$(grep -P '^\t' go.mod) # Uses Perl-style regular expressions to match a tab at the beginning of a line.
gardener_go_mod=$(curl -s https://raw.githubusercontent.com/gardener/gardener/refs/heads/master/go.mod | grep -P '^\t')

local_dependencies=()
gardener_dependencies=()

extract_dependencies "$local_go_mod" local_dependencies
extract_dependencies "$gardener_go_mod" gardener_dependencies

echo "📜 Found ${#local_dependencies[@]} local dependencies."
echo "🚜 Found ${#gardener_dependencies[@]} gardener dependencies."

# Extract the intersection of the two arrays by iterating over them in a nested fashion.
common_dependencies=()

for local_dependency in "${local_dependencies[@]}"; do
    for gardener_dependency in "${gardener_dependencies[@]}"; do
        if [[ "$local_dependency" == "$gardener_dependency" ]]; then
            common_dependencies+=("$local_dependency")
            break # Continue with the next element of the outer loop.
        fi
    done
done

echo "☯️ Found ${#common_dependencies[@]} common dependencies."

ignore_deps=$(printf ',"%s"' "${common_dependencies[@]}") # Add a comma to the beginning of each element and concatenate them.
ignore_deps="[${ignore_deps:1}]" # Remove the leading comma and wrap the string in square brackets to format it as a JSON array.

# Format the JSON array as a string, indent it, and use sed to replace the lines between the markers
echo "$ignore_deps" | yq -o json '.[]' | sed 's/^/    /; s/$/,/' | sed -i -e '  /  ignoreDeps: \[/,  /\]/{//!d;}' -e '  /  ignoreDeps: \[/r /dev/stdin' renovate.json5
