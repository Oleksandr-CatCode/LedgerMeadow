"""Check credential handling with a synthetic environment and a fake psql process."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class LocalDatabaseScriptTests(unittest.TestCase):
    def run_script(self, root, admin_host='127.0.0.1', fail=False):
        scripts = root / 'scripts'
        scripts.mkdir()
        shutil.copyfile(ROOT / 'scripts/create-local-db.sh', scripts / 'create-local-db.sh')
        api = root / 'apps/api'
        api.mkdir(parents=True)
        (api / '.env.local').write_text(
            f'POSTGRES_ADMIN_URL=postgresql://test_user:synthetic-password@{admin_host}:5432/postgres\n'
            'DATABASE_URL=postgresql://test_user:synthetic-password@127.0.0.1:5432/synthetic_finance\n')
        bin_dir = root / 'bin'
        bin_dir.mkdir()
        stub = bin_dir / 'psql'
        stub.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
pathlib.Path(os.environ['CAPTURE']).write_text(json.dumps({'args':sys.argv[1:], 'database':os.environ['PGDATABASE'], 'password':os.environ['PGPASSWORD'], 'sql':sys.stdin.read()}))
if os.environ.get('FAKE_FAIL') == '1':
    print('synthetic-password database diagnostics', file=sys.stderr)
    sys.exit(1)
''')
        stub.chmod(0o755)
        env = os.environ.copy()
        env.update(PATH=str(bin_dir)+os.pathsep+env['PATH'], CAPTURE=str(root/'capture.json'), FAKE_FAIL=str(int(fail)))
        return subprocess.run(['bash', str(scripts/'create-local-db.sh')], env=env, capture_output=True, text=True)

    def test_credentials_do_not_enter_arguments_or_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result = self.run_script(root)
            self.assertEqual(result.returncode, 0, result.stderr)
            capture = json.loads((root/'capture.json').read_text())
            self.assertEqual(capture['password'], 'synthetic-password')
            self.assertEqual(capture['database'], 'postgres')
            self.assertNotIn('synthetic-password', json.dumps(capture['args']) + result.stdout + result.stderr)
            self.assertIn("format('CREATE DATABASE %I'", capture['sql'])

    def test_database_diagnostics_are_not_echoed(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = self.run_script(Path(tmp), fail=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn('synthetic-password', result.stdout + result.stderr)

    def test_nonlocal_administrator_is_rejected_without_connecting(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result = self.run_script(root, admin_host='db.example.com')
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse((root/'capture.json').exists())


if __name__ == '__main__':
    unittest.main()
