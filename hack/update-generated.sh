#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

rm -rf pkg/generated

bash vendor/k8s.io/code-generator/generate-groups.sh \
deepcopy,client,lister,informer \
github.com/openshift/csi-driver-shared-resource/pkg/generated \
github.com/openshift/csi-driver-shared-resource/pkg/api \
sharedresource:v1alpha1 \
--go-header-file "./hack/boilerplate.go.txt"