#!/bin/bash

args=("-f" "--kty=OKP" "--crv=Ed25519" "--insecure" "--no-password")

step certificate create "Example Root CA" root_ca.crt root_ca.key --profile=root-ca "${args[@]}"
step certificate create "Example Intermediate CA 1" intermediate_ca_1.crt intermediate_ca_1.key --profile=intermediate-ca --ca=root_ca.crt --ca-key=root_ca.key "${args[@]}"
step certificate create "Example Intermediate CA 2" intermediate_ca_2.crt intermediate_ca_2.key --profile=intermediate-ca --ca=intermediate_ca_1.crt --ca-key=intermediate_ca_1.key "${args[@]}"
step certificate create "Example Intermediate CA 3" intermediate_ca_3.crt intermediate_ca_3.key --profile=intermediate-ca --ca=intermediate_ca_2.crt --ca-key=intermediate_ca_2.key "${args[@]}"
step certificate create "example.com" example.com.crt example.com.key --profile=leaf --ca=intermediate_ca_3.crt --ca-key=intermediate_ca_3.key "${args[@]}"
step certificate create "leaf" localhost.crt localhost.key --profile=leaf --ca=root_ca.crt --ca-key=root_ca.key --san=localhost "${args[@]}"
step certificate create "self-signed-leaf" self_signed_leaf.crt self_signed_leaf.key --profile=self-signed --subtle "${args[@]}"

cat example.com.crt intermediate_ca_3.crt intermediate_ca_2.crt intermediate_ca_1.crt root_ca.crt >full_chain.crt

jsonData='{"testData":[]}'
for f in example.com.crt intermediate_ca_3.crt intermediate_ca_2.crt intermediate_ca_1.crt root_ca.crt; do
  sha256="sha256:$(openssl x509 -in $f -pubkey | openssl pkey -pubin -outform der 2>/dev/null | openssl dgst -binary | base64 | tr '/+' '_-' | tr -d '=')"
  b2b256="b2b256:$(openssl x509 -in $f -pubkey | openssl pkey -pubin -outform der 2>/dev/null | b2sum -l 256 | xxd -r -p | base64 | tr '/+' '_-' | tr -d '=')"
  jsonData=$(jq -c --arg sha256 "$sha256" --arg b2b256 "$b2b256" --arg f "$f" '.testData[.testData | length] |= . + {"cert":$f,"fingerprints":{"sha256":$sha256,"b2b256":$b2b256}}' <<<"$jsonData")
done
jq <<<"$jsonData" >fingerprints.json

cd cortex || exit 1
step certificate create "Test Cortex CA" root.crt root.key --profile=root-ca "${args[@]}"
step certificate create "Test Cortex Client" client.crt client.key --profile=leaf --ca=root.crt --ca-key=root.key --san=localhost "${args[@]}"
step certificate create "Test Cortex Server" server.crt server.key --profile=leaf --ca=root.crt --ca-key=root.key --san=localhost "${args[@]}"
