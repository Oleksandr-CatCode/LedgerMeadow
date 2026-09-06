#!/usr/bin/env bash

set -euo pipefail

readonly protected_branch="${PROTECTED_BRANCH:-main}"
readonly operation="${1:-commit}"

current_branch="$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)"

deny() {
  printf 'Blocked: %s is protected. Create a feature branch and open a pull request instead.\n' "${protected_branch}" >&2
  exit 1
}

case "${operation}" in
  commit)
    if [[ "${current_branch}" == "${protected_branch}" ]] && git rev-parse --verify HEAD >/dev/null 2>&1; then
      deny
    fi
    ;;
  merge)
    if [[ "${current_branch}" == "${protected_branch}" ]]; then
      deny
    fi
    ;;
  push)
    while read -r _local_ref _local_sha remote_ref _remote_sha; do
      if [[ "${remote_ref}" == "refs/heads/${protected_branch}" ]]; then
        deny
      fi
    done
    ;;
  *)
    printf 'Unknown protected-branch operation: %s\n' "${operation}" >&2
    exit 2
    ;;
esac
