#!/usr/bin/env python3
"""Check the standalone SDK contract without the CPA host dependency graph."""
import os
import subprocess
subprocess.run(['go', 'test', './...'], cwd='sdk/protocol', env={**os.environ, 'GOWORK': 'off'}, check=True)
