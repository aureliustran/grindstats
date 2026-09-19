#!/usr/bin/env bash
# gen-jwt-keys.sh — generate a local RS256 keypair for development.
#
# Output: .local/jwt/signing.key  (private key, PEM PKCS#8)
#         .local/jwt/public/<kid>.pub  (matching public key, PEM PKIX)
#
# The kid is derived from the SHA-256 fingerprint of the public key so that
# rotating keys (by re-running this script with a different KID_PREFIX) never
# silently reuses a kid from a previous run.
#
# SEC-06: .local/ is git-ignored. Never commit the generated keys.
# Run via Git Bash: bash scripts/dev/gen-jwt-keys.sh
#
# Dependencies: openssl (bundled with Git for Windows / Git Bash)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KEY_DIR="${REPO_ROOT}/.local/jwt"
PUB_DIR="${KEY_DIR}/public"
PRIV_KEY="${KEY_DIR}/signing.key"

mkdir -p "${PUB_DIR}"

echo "Generating 4096-bit RSA private key..."
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out "${PRIV_KEY}" 2>/dev/null

# Extract the public key.
TMP_PUB="${KEY_DIR}/tmp_pub.pem"
openssl pkey -in "${PRIV_KEY}" -pubout -out "${TMP_PUB}" 2>/dev/null

# Derive a stable kid from the DER fingerprint of the public key.
# The kid is the first 16 hex characters of the SHA-256 digest of the DER-
# encoded SubjectPublicKeyInfo blob — enough to be unique across rotations
# without being unwieldy.
DER_HEX="$(openssl pkey -in "${PRIV_KEY}" -pubout -outform DER 2>/dev/null | sha256sum | cut -c1-16)"
KID="key-${DER_HEX}"

PUB_KEY="${PUB_DIR}/${KID}.pub"
mv "${TMP_PUB}" "${PUB_KEY}"

echo "Private key : ${PRIV_KEY}"
echo "Public key  : ${PUB_KEY}  (kid=${KID})"
echo ""
echo "Add to your .env (or docker-compose.yml environment section):"
echo "  AUTH_JWT_PRIVATE_KEY_PATH=.local/jwt/signing.key"
echo "  AUTH_JWT_PUBLIC_KEYS_DIR=.local/jwt/public"
echo ""
echo "Generate a random OAuth state key (32 bytes, hex-encoded) and add:"
echo "  AUTH_GOOGLE_STATE_KEY=$(openssl rand -hex 32)"
echo ""
echo "Done. .local/ is git-ignored; never commit these files."
