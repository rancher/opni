#!/bin/bash

rm -f root_ca.crt root_ca.key leaf.crt leaf.key
step certificate create root-ca --profile root-ca root_ca.crt root_ca.key --kty OKP --curve Ed25519 --insecure --no-password
step certificate create leaf --profile leaf --ca root_ca.crt --ca-key root_ca.key leaf.crt leaf.key --kty OKP --curve Ed25519 --insecure --no-password
