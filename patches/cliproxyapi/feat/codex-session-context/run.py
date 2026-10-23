#!/usr/bin/env python3
"""Verify the codex-session-context bundle against the complete patched host."""
import subprocess

subprocess.run(['go', 'test', '-tags', 'cpa_hold_buffer', '-p', '1', './internal/guardhttp', './internal/runtime/executor/helps', './internal/runtime/executor', './sdk/api/handlers', './sdk/api/handlers/openai'], check=True)
