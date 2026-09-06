#!/usr/bin/env bash
set -euo pipefail
set -m

readonly repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
readonly api_env="${repository_root}/apps/api/.env.local"
readonly web_env="${repository_root}/apps/web/.env.local"
if [[ ! -f "${api_env}" || ! -f "${web_env}" ]]; then
  printf 'Create the private API and web environment files from their examples first.\n' >&2
  exit 1
fi

(cd "${repository_root}/apps/financial-engine" && cargo build --locked --bin financial-engine-server)
(
  set -a
  # shellcheck disable=SC1090
  source "${api_env}"
  set +a
  cd "${repository_root}/apps/financial-engine"
  exec env -i PATH="$PATH" HOME="$HOME" FINANCIAL_ENGINE_ADDR="${FINANCIAL_ENGINE_ADDR:?}" ./target/debug/financial-engine-server
) &
engine_pid=$!
(
  set -a
  # shellcheck disable=SC1090
  source "${api_env}"
  set +a
  cd "${repository_root}/apps/api"
  exec go run .
) &
api_pid=$!
(
  set -a
  # shellcheck disable=SC1090
  source "${web_env}"
  set +a
  cd "${repository_root}/apps/web"
  exec env -i PATH="$PATH" HOME="$HOME" VITE_CLERK_PUBLISHABLE_KEY="${VITE_CLERK_PUBLISHABLE_KEY:?}" VITE_API_BASE_URL="${VITE_API_BASE_URL:-http://127.0.0.1:8080}" npm run dev -- --host 127.0.0.1
) &
web_pid=$!
cleanup() {
  kill -- "-${engine_pid}" "-${api_pid}" "-${web_pid}" 2>/dev/null || true
  wait "${engine_pid}" "${api_pid}" "${web_pid}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
wait -n "${engine_pid}" "${api_pid}" "${web_pid}"
