#!/usr/bin/env python3
"""Verify CPA source identity and globally unique patch ownership without network."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
output = parser.add_mutually_exclusive_group()
output.add_argument('--list-patches', action='store_true')
output.add_argument('--list-bundles', action='store_true')
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
manifest = json.loads((root / 'patches/cliproxyapi/manifest.json').read_text())
relationship = json.loads((root / 'docs/source-relationships/cliproxyapi-materialization.json').read_text())
if (manifest['source_relationship'] != 'docs/source-relationships/cliproxyapi-materialization.json'
        or relationship['upstream'] != {k: manifest['upstream'][k] for k in ('url', 'commit')}
        or relationship['result_tree'] != manifest['result_tree']):
    raise SystemExit('source relationship differs from the materialization manifest')
if (relationship['schema_version'] != 'git-source-relationship/v4'
        or relationship['kind'] != 'patched_upstream_build'
        or relationship['integration'] != 'build'
        or relationship['path'] != 'build/cliproxyapi'
        or relationship['build_path'] != relationship['path']
        or manifest['output'] != relationship['build_path']):
    raise SystemExit('CPA must use an upstream-authoritative build relationship')
for oid in (relationship['upstream']['commit'], manifest['upstream']['tree'], relationship['result_tree']):
    if not re.fullmatch(r'[0-9a-f]{40}', oid):
        raise SystemExit('source and result identities must be full SHA-1 object IDs')

bundles = relationship['patch_bundles']
if not bundles or len(set(bundles)) != len(bundles):
    raise SystemExit('patch bundles must be nonempty and unique')
owners = {}
patches = []
for bundle_name in bundles:
    bundle = Path(bundle_name)
    if (len(bundle.parts) != 4 or bundle.parts[0] != 'patches'
            or bundle.parts[1] not in ('cliproxyapi', 'cliproxyapi-sdk')
            or any(not re.fullmatch(r'[a-z0-9]+(?:-[a-z0-9]+)*', part) for part in bundle.parts)):
        raise SystemExit(f'invalid semantic bundle: {bundle_name}')
    runner = root / bundle / 'run.py'
    compile(runner.read_text(encoding='utf-8'), str(runner), 'exec')
    order_text = (root / bundle / 'apply_order.txt').read_text(encoding='utf-8')
    if not order_text.endswith('\n'):
        raise SystemExit(f'order file must end with a newline: {bundle_name}')
    members = {'apply_order.txt', 'run.py'}
    for order, line in enumerate(order_text.splitlines(), 1):
        match = re.fullmatch(r'([1-9][0-9]*) ([A-Za-z0-9._-]+\.patch) ([0-9a-f]{64})', line)
        if match is None or int(match[1]) != order or match[2] in members:
            raise SystemExit(f'invalid patch order: {bundle_name}: {line}')
        filename, digest = match[2], match[3]
        path = bundle / filename
        if hashlib.sha256((root / path).read_bytes()).hexdigest() != digest:
            raise SystemExit(f'patch checksum mismatch: {path}')
        stats = subprocess.check_output(['git', 'apply', '--numstat', str(root / path)], text=True).splitlines()
        if len(stats) != 1:
            raise SystemExit(f'patch must modify exactly one file: {path}')
        target = stats[0].split('\t')[-1]
        if target.replace('/', '__') + '.patch' != filename:
            raise SystemExit(f'patch filename does not identify its target: {path}')
        if target in owners:
            raise SystemExit(f'patch-over-patch: {target}: {owners[target]} and {path}')
        owners[target] = path.as_posix()
        patches.append((path.as_posix(), digest))
        members.add(filename)
    if len(members) == 2 or {p.name for p in (root / bundle).iterdir()} != members:
        raise SystemExit(f'empty bundle or unlisted bundle member: {bundle_name}')

actual = {p.relative_to(root).as_posix() for family in ('cliproxyapi', 'cliproxyapi-sdk')
          for p in (root / 'patches' / family).rglob('*.patch')}
if actual != {path for path, _ in patches}:
    raise SystemExit('patch roots have unlisted or missing patches')
if args.list_patches:
    for path, digest in patches:
        print(f'{path}\t{digest}')
elif args.list_bundles:
    print('\n'.join(bundles))
else:
    print(f'Verified {len(bundles)} bundles; {len(patches)} patches; each target has one owner')
