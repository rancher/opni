#!/bin/bash

dirs=$(go list -f "{{ if ne .Name \"main\" }}{{ if and (not (len .XTestGoFiles)) (not (len .TestGoFiles)) }}{{ .Dir }}{{ end }}{{ end }}" ./... | grep -v "pkg/apis")

# for each directory that needs bootstrapping, run 'ginkgo bootstrap' in the directory
for dir in $dirs; do
  pushd $dir &>/dev/null
  echo "$dir"
  ginkgo bootstrap
  popd &>/dev/null
done
