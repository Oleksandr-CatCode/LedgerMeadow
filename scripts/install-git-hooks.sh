#!/usr/bin/env bash

set -euo pipefail

repository_root="$(git rev-parse --show-toplevel 2>/dev/null)" || {
  printf 'Run this script from inside the Smart Money Hub Git repository.\n' >&2
  exit 1
}

git -C "${repository_root}" config core.hooksPath .githooks

printf 'Installed local Git hooks. Direct commits, merge commits, and pushes to main are blocked.\n'
printf 'Remote branch protection must also be enabled on the Git hosting provider.\n'
