#!/usr/bin/env python3
"""Verify the infra-diagnostics bundle against the complete patched host."""
import subprocess

subprocess.run(['go', 'test', '-tags', 'cpa_hold_buffer', '-p', '1', './internal/logging'], check=True)
