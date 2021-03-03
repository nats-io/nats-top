#!/bin/sh -eu
set -eu

# This script is a convenience wrapper for making fresh test data with no
# extra dependencies.  We create in the current working directory.
#
# Most people should not need to run this; it's an "every few years" maintainer
# task.  So we can depend upon specific tools which make it sane to manage
# extensions.
# We're using github.com/cloudflare/cfssl

progname="$(basename "$0" .sh)"
warn() { printf >&2 '%s: %s\n' "$progname" "$*"; }
die() { warn "$@"; exit 1; }

[ -f tls.conf ] || die "no tls.conf in current directory"

ORG_NAME='Synadia Communications, Inc'
CA_COMMON_NAME='NATS top tool test suite'

# The default duration of the CA in cfssl is 5 years, changing it is
# non-obvious, so let's cap the lifetime of the issued certs to the CA duration
# to avoid confusion when the CA expires.
DURATION_YEARS=5
DURATION_STR="$(( DURATION_YEARS * 365 * 24 ))h"

csr_filter_common() {
  jq --arg org "$ORG_NAME" -r 'del(.CN) | del(.names[0].L) | .names[0].O=$org'
}

ca_csr_json() {
  cfssl print-defaults csr | csr_filter_common | \
    jq --arg cn "$CA_COMMON_NAME" -r '.CN=$cn | del(.hosts)'
}

client_csr_json() {
  cfssl print-defaults csr | csr_filter_common | \
    jq -r 'del(.hosts)'
}

server_csr_json() {
  cfssl print-defaults csr | csr_filter_common | \
    jq -r '.hosts = ["localhost", "127.0.0.1", "::1"]'
}

ca_cfg_json() {
  cfssl print-defaults config | \
    jq --arg dur "$DURATION_STR" -r '.signing.profiles |= map_values(.expiry = $dur)'
}

KEY_CURVE=secp384r1
SUBJ_PREFIX='/C=US/ST=California/O=Synadia Communications, LLC/OU=NATS'

CA_PUBLIC=ca.pem
# We will delete the key when done
CA_KEY=ca-key.pem
CA_CFG=ca-config.json

ca_cfg_json > "./$CA_CFG"

ca_csr_json | cfssl genkey -initca - | cfssljson -bare ca

gencert() { cfssl gencert -ca "$CA_PUBLIC" -ca-key "$CA_KEY" -config "$CA_CFG" "$@" ; }

client_csr_json | gencert -profile=client - | cfssljson -bare client
server_csr_json | gencert -profile=www - | cfssljson -bare server
mv -v client.pem client-cert.pem
mv -v server.pem server-cert.pem

rm -v ./*.csr "$CA_KEY" "$CA_CFG"
