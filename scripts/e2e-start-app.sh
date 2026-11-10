#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${MPLATFORM_E2E_PORT:-38080}"
if [ -n "${MPLATFORM_E2E_DB_PATH:-}" ]; then
  DB_PATH="${MPLATFORM_E2E_DB_PATH}"
else
  DB_PATH="$(mktemp "${TMPDIR:-/tmp}/mplatform-e2e.XXXXXX")"
fi

cd "${ROOT_DIR}"

rm -f "${DB_PATH}"

make build

export DB_TYPE=sqlite
export DB_DSN="${DB_PATH}"
export DISABLE_VERSION_CHECK=true
export JWT_SECRET="${JWT_SECRET:-mplatform-e2e-jwt-secret}"
export MPLATFORM_ENCRYPT_KEY="${MPLATFORM_ENCRYPT_KEY:-mplatform-e2e-encryption-key}"
export PORT
export HOME="${MPLATFORM_E2E_HOME:-$(mktemp -d "${TMPDIR:-/tmp}/mplatform-e2e-home.XXXXXX")}"
unset MPLATFORM_USERNAME
unset MPLATFORM_PASSWORD
unset KUBECONFIG

exec ./mplatform -v 3
