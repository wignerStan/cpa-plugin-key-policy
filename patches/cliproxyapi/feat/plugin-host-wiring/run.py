#!/usr/bin/env python3
"""Verify the plugin-host-wiring bundle against the complete patched host."""
import subprocess

subprocess.run(['go', 'test', '-tags', 'cpa_hold_buffer', '-p', '1', './cmd/server', './internal/api', './internal/config', './internal/api/handlers/management', './internal/pluginhost', './sdk/cliproxy', './sdk/config'], check=True)
