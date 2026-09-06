#!/usr/bin/env python3
"""Check every tracked publication file without printing sensitive values."""
from pathlib import Path
import hashlib
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
SYNTHETIC_CSV = 'apps/api/src/modules/transactionimports/validators/testdata/rbc_chequing.csv'
# Exact browser captures reviewed for synthetic pixels and non-personal metadata.
REVIEWED_IMAGES = {
    'docs/screenshots/dashboard.jpg': '52e8aeebebb607fb3f7240e6de6b3fce3adbcb86bfa5b7cfc83e683dff131ffc',
    'docs/screenshots/accounts.jpg': '4215864f4fcfc808f78d181a3611208dbb5ad0db4b62e8ab560932a4c62a3d7a',
    'docs/screenshots/spaces.jpg': '2369a90732399f5a4c96a24222d2aacca544712b3f35e0a86abd03feb95f0787',
    'docs/screenshots/planning.jpg': '351171a96d512b0e8f9c216604745fbe586d95220a93f25234aba8a9923c1af4',
}
NUMERIC_BOUNDARIES = {'9223372036854775807', '9223372036854775808',
                      '9007199254740991', '9007199254740992', '9007199254740993'}
BLOCKED_DIRS = {'.agents', '.claude', '.local-secrets', '.secrets', '.aws', '.ssh', '.modal',
                'node_modules', 'target', 'dist', 'build', 'coverage', '.cache', '.venv',
                'venv', '__pycache__', 'data', 'exports', 'uploads', 'backups', 'seeds'}
BLOCKED_SUFFIXES = {'.key', '.pem', '.p12', '.pfx', '.keystore', '.local', '.csv', '.tsv',
                    '.xlsx', '.xls', '.pdf', '.ofx', '.qfx', '.qif', '.db', '.sqlite', '.sqlite3',
                    '.dump', '.backup', '.bak', '.zip', '.gz', '.tar', '.log', '.map', '.pyc',
                    '.png', '.jpg', '.jpeg', '.webp', '.gif', '.pt', '.pth', '.npy', '.npz'}
entries = subprocess.check_output(['git', 'ls-files', '-s', '-z'], cwd=ROOT).decode().split('\0')[:-1]
if not entries:
    sys.exit('Stage the intended publication files before checking.')
errors = []
for entry in entries:
    meta, name = entry.split('\t', 1)
    mode, oid, stage = meta.split()
    path = Path(name)
    lower_name = path.name.lower()
    is_reviewed_image = name in REVIEWED_IMAGES
    if mode not in {'100644', '100755'} or stage != '0':
        errors.append(f'{name}: symlink, nested repository, or unresolved entry')
    is_example = path.name == '.env.example'
    blocked_env = lower_name.startswith('.env') and not is_example
    if ({part.lower() for part in path.parts} & BLOCKED_DIRS or lower_name in {'.npmrc', '.envrc', '.modal.toml'}
        or blocked_env or (path.suffix.lower() in BLOCKED_SUFFIXES and name != SYNTHETIC_CSV and not is_reviewed_image)):
        errors.append(f'{name}: private or generated file category')
    data = subprocess.check_output(['git', 'cat-file', 'blob', oid], cwd=ROOT)
    if not (ROOT/name).is_file() or (ROOT/name).read_bytes() != data:
        errors.append(f'{name}: working file differs from staged file; stage and recheck')
    if len(data) > 2_000_000:
        errors.append(f'{name}: oversized source file requires review')
    if is_reviewed_image:
        if hashlib.sha256(data).hexdigest() != REVIEWED_IMAGES[name]:
            errors.append(f'{name}: screenshot bytes changed; repeat visual and metadata privacy review')
        continue
    try:
        content = data.decode('utf-8')
    except UnicodeError:
        errors.append(f'{name}: binary content is not allowed')
        continue
    for line_no, line in enumerate(content.splitlines(), 1):
        number_line = line
        if path.name in {'go.mod', 'go.sum'}:
            number_line = re.sub(r'\bv[0-9]+\.[0-9]+\.[0-9]+-(?:[0-9A-Za-z.-]*[.-])?[0-9]{14}-[0-9a-f]{12}\b', 'GO_PSEUDOVERSION', line)
        for number in re.findall(r'(?<![A-Za-z0-9_])[0-9]{13,19}(?![A-Za-z0-9_])', number_line):
            if set(number) != {'0'} and number not in NUMERIC_BOUNDARIES:
                errors.append(f'{name}:{line_no}: long financial-identifier-shaped number requires replacement')
        if re.search(r'/(?:home|Users)/[A-Za-z0-9_.-]+/', line):
            errors.append(f'{name}:{line_no}: personal filesystem path')
        if re.search(r'(?i)https?://[^\s/"\x27]*(?:r2\.cloudflarestorage\.com|r2\.dev|modal\.run|modal\.host|modal\.direct|neon\.tech|clerk\.accounts\.dev)', line):
            errors.append(f'{name}:{line_no}: private deployment endpoint')
        if is_example:
            match = re.match(r'([A-Z_][A-Z_0-9]*)=(.*)', line)
            if match:
                key, value = match[1], match[2].strip().strip('\"\x27')
                sensitive = any(part in key for part in ('PASSWORD', 'SECRET', 'TOKEN', 'CLIENT_ID', 'PUBLISHABLE_KEY', 'DATABASE', 'ADMIN_URL'))
                if sensitive and not key.endswith('_FILE') and value:
                    errors.append(f'{name}:{line_no}: credential example must be empty')
    if path.suffix.lower() == '.svg':
        try:
            svg = ET.fromstring(content)
            for node in svg.iter():
                if node.tag.split('}')[-1] not in {'svg', 'rect', 'path'} or any(k.lower().startswith('on') or k.endswith('href') for k in node.attrib):
                    errors.append(f'{name}: unexpected SVG payload or metadata')
        except ET.ParseError:
            errors.append(f'{name}: malformed SVG')
if errors:
    sys.exit('\n'.join(errors))
print(f'Publication checks passed: {len(entries)} tracked files; only reviewed screenshots may contain binary data.')
