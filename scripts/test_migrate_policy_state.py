import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import yaml

spec = importlib.util.spec_from_file_location('migration', Path(__file__).with_name('migrate-policy-state.py'))
migration = importlib.util.module_from_spec(spec)
spec.loader.exec_module(migration)


class MigrationTests(unittest.TestCase):
    def test_all_settings_move_and_usage_survives(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            state = root / 'state.json'
            config = root / 'config.yaml'
            state.write_text(json.dumps({'version': 2, 'usage': {'k': {'daily': {'total_usd': 8}}}}))
            settings = {'enabled': True, 'priority': 10, 'state_file': str(state),
                        'keys': [{'id': 'k', 'enabled': True, 'key': 'temporary-secret'}],
                        'catalog_groups': [{'name': 'gpt', 'keys': ['k'], 'include_models': ['gpt-*']}],
                        'global_weighted_round_robin': True,
                        'aliases': [], 'classify_rules': []}
            config.write_text(yaml.safe_dump({'plugins': {'enabled': True, 'dir': 'plugins',
                                                        'configs': {'cpa-key-policy': settings}}}))
            migration.migrate(config)
            doc = json.loads(state.read_text())
            registration = yaml.safe_load(config.read_text())['plugins']['configs']['cpa-key-policy']
            self.assertEqual(set(registration), {'enabled', 'priority', 'state_file', 'poll_interval'})
            self.assertEqual(doc['usage']['k']['daily']['total_usd'], 8)
            self.assertEqual(doc['catalog_groups'], settings['catalog_groups'])
            self.assertTrue(doc['global_weighted_round_robin'])
            self.assertTrue(doc['keys'][0]['key_hash'].startswith('sha256:'))
            self.assertNotIn('temporary-secret', state.read_text())
            before = copy.deepcopy(doc)
            migration.migrate(config)
            self.assertEqual(json.loads(state.read_text()), before)


if __name__ == '__main__':
    unittest.main()
