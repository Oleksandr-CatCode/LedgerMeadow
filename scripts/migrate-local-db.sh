#!/usr/bin/env bash

set -euo pipefail

readonly repository_root="$(
  cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
  pwd -P
)"
readonly api_env="${repository_root}/apps/api/.env.local"

if [[ ! -f "${api_env}" ]]; then
  printf 'Missing %s. Copy .env.example and provide local secrets first.\n' "${api_env}" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${api_env}"
set +a

# The same runner ships in the production image, so local and deployed migrations
# take an identical code path.
cd "${repository_root}/apps/api"
MIGRATIONS_DIR="${repository_root}/db/migrations" exec go run ./cmd/migrate
