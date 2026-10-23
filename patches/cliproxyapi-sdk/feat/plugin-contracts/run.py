#!/usr/bin/env python3
"""Verify the plugin-contracts bundle against the complete patched host."""
import subprocess

subprocess.run(['go', 'test', '-tags', 'cpa_hold_buffer', '-p', '1', './sdk/pluginapi', './sdk/pluginabi', './sdk/pluginhost', './sdk/httptransport'], check=True)
