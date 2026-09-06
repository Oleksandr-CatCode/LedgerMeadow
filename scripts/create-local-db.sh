#!/usr/bin/env bash
set -euo pipefail
readonly repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
readonly api_env="${repository_root}/apps/api/.env.local"
if [[ ! -f "${api_env}" ]]; then
  printf 'Create apps/api/.env.local from its example first.\n' >&2
  exit 1
fi
set -a
# shellcheck disable=SC1090
source "${api_env}"
set +a
# libpq reads the administrator connection from the environment, never process arguments.
# SQL variables are quoted by psql, not interpolated by the shell.
python3 - <<'PY'
import os, re, subprocess
from urllib.parse import urlsplit, unquote
admin = os.environ.get('POSTGRES_ADMIN_URL', '')
target = os.environ.get('DATABASE_URL', '')
try:
    admin_url, target_url = urlsplit(admin), urlsplit(target)
    local_hosts = {'localhost', '127.0.0.1', '::1'}
    if any(u.scheme not in {'postgres', 'postgresql'} or u.hostname not in local_hosts for u in (admin_url, target_url)):
        raise ValueError()
    name = unquote(target_url.path.lstrip('/'))
    if not re.fullmatch(r'[A-Za-z_][A-Za-z_0-9]{0,62}', name):
        raise ValueError()
except ValueError:
    raise SystemExit('Configure valid loopback PostgreSQL URLs and a simple database name in the private API environment.')
env = os.environ.copy()
env.update(PGHOST=admin_url.hostname, PGPORT=str(admin_url.port or 5432),
           PGDATABASE=unquote(admin_url.path.lstrip('/')) or 'postgres',
           PGUSER=unquote(admin_url.username or ''), PGPASSWORD=unquote(admin_url.password or ''))
query = "SELECT format('CREATE DATABASE %I', :'database_name') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = :'database_name')\\gexec\n"
result = subprocess.run(['psql', '--no-psqlrc', '--set=ON_ERROR_STOP=1', '--set=database_name='+name], input=query, text=True, env=env, capture_output=True)
if result.returncode:
    raise SystemExit('Local database creation failed. Check your private PostgreSQL settings.')
print('Local development database is ready.')
PY
