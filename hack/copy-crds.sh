#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -e

source_dir="$(dirname "$0")/../pkg/apis/dns/crds"
destination="$(dirname "$0")/../charts/external-dns-management/templates/crds.yaml"

# Function to update the metadata section and copy to destination
update_and_copy() {
    local source_file="$1"
    local dest_file="$destination_dir/$(basename "$source_file")"

    # Use awk to update the metadata and copy to destination
    awk '/^metadata:/ {
        print
        metadata_found = 1
        if ($0 == "metadata:") {
            print "  labels:"
            print "    helm.sh/chart: {{ include \"external-dns-management.chart\" . }}"
            print "    app.kubernetes.io/name: {{ include \"external-dns-management.name\" . }}"
            print "    app.kubernetes.io/instance: {{ .Release.Name }}"
            print "    app.kubernetes.io/managed-by: {{ .Release.Service }}"
        }
        next
    }
    metadata_found == 1 { metadata_found = 0 }
    {print}' "$source_file" >> "$destination"
}

echo '{{- if .Values.createCRDs }}' > "$destination"
# Iterate through each YAML file in the source directory
files=(
     "dns.gardener.cloud_dnsentries.yaml"
     "dns.gardener.cloud_dnsannotations.yaml"
     "dns.gardener.cloud_dnsowners.yaml"
     "dns.gardener.cloud_dnsproviders.yaml"
     "dns.gardener.cloud_dnshostedzonepolicies.yaml"
     "dns.gardener.cloud_dnslocks.yaml"
)
for filename in "${files[@]}"
do
     source_file="$source_dir/$filename"
     if [ -f "$source_file" ]; then
         update_and_copy "$source_file"
     fi
done
echo '{{- end }}' >> "$destination"
