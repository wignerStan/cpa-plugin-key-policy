#!/usr/bin/env python3
"""Generate or verify the byte-identical OS notification kit from committed infra."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent.parent
manifest_path = root / 'docs/source-relationships/infra-file-notifications.json'
relative = 'internal/infra/files/notify/file_subscription.go'
source_path = 'go/files/notify/file_subscription.go'
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--update-from', type=Path)
args = parser.parse_args()
if args.update_from:
    commit = subprocess.check_output(['git', '-C', str(args.update_from), 'rev-parse', 'HEAD'], text=True).strip()
    raw = subprocess.check_output(['git', '-C', str(args.update_from), 'show', f'{commit}:{source_path}'])
    target = root / relative
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(raw)
    manifest_path.write_text(json.dumps({'repository': 'https://github.com/wignerStan/infra',
                                        'commit': commit, 'source_path': source_path,
                                        'generated_path': relative,
                                        'sha256': hashlib.sha256(raw).hexdigest()}, indent=2) + '\n')
manifest = json.loads(manifest_path.read_text())
path = root / manifest['generated_path']
if path.is_symlink() or hashlib.sha256(path.read_bytes()).hexdigest() != manifest['sha256']:
    raise SystemExit('infra file subscription snapshot verification failed')
print('Verified byte-identical infra OS file subscription snapshot')
