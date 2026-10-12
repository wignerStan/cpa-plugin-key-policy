#!/usr/bin/env python3
"""Stop CPA first. Move all plugin settings from YAML into authoritative JSON."""
import argparse
import datetime
import hashlib
import json
import os
import shutil
from pathlib import Path

import yaml


def migrate(config_path):
    path = Path(config_path)
    config = yaml.safe_load(path.read_text())
    plugin = config['plugins']['configs']['cpa-key-policy']
    state_path = Path(plugin.get('state_file', 'cpa-key-policy-state.json'))
    if not state_path.is_absolute():
        raise ValueError('Use an absolute state_file path before migration')
    state = json.loads(state_path.read_text()) if state_path.exists() else {}
    registration_fields = {'enabled', 'priority', 'state_file'}
    settings = {key: value for key, value in plugin.items() if key not in registration_fields}
    if state.get('version', 0) >= 3 and not settings:
        print('Plugin settings already live in JSON')
        return
    if state.get('version', 0) >= 3 and settings:
        raise ValueError('JSON is already authoritative; resolve leftover YAML settings explicitly')
    stamp = datetime.datetime.now().strftime('%Y%m%dT%H%M%S')
    for source in [path, state_path]:
        if source.exists():
            backup = str(source) + '.pre-json-settings-' + stamp
            shutil.copy2(source, backup)
            os.chmod(backup, 0o600)
    # YAML was authoritative in version 2. In version 1, keep existing JSON
    # keys and let JSON win policy conflicts, matching the old loading rules.
    if state.get('version') == 2:
        state.update(settings)
    else:
        for key, value in settings.items():
            if key == 'catalog_groups' or key not in state:
                state[key] = value
    state.setdefault('enabled', plugin.get('enabled', True))
    state.setdefault('global_weighted_round_robin', False)
    state.setdefault('keys', [])
    state.setdefault('aliases', [])
    state.setdefault('classify_rules', [])
    state.setdefault('catalog_groups', [])
    state.setdefault('usage', {})
    for key in state['keys']:
        secret = key.pop('key', None)
        key.pop('key_preview', None)
        if secret:
            key['key_hash'] = secret if secret.startswith('sha256:') else 'sha256:' + hashlib.sha256(secret.strip().encode()).hexdigest()
    state['version'] = 3
    state['updated_at'] = datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z')
    config['plugins']['configs']['cpa-key-policy'] = {
        'enabled': plugin.get('enabled', True),
        'priority': plugin.get('priority', 10),
        'state_file': str(state_path),
    }
    for target, content in [(state_path, json.dumps(state, indent=2) + '\n'),
                            (path, yaml.safe_dump(config, sort_keys=False, allow_unicode=True))]:
        target.parent.mkdir(parents=True, exist_ok=True)
        tmp = target.with_name(target.name + '.migration.tmp')
        with tmp.open('w') as stream:
            os.chmod(tmp, 0o600)
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(tmp, target)
    print('Moved all plugin settings to JSON; YAML retains registration only')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('config')
    migrate(parser.parse_args().config)
