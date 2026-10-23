#!/usr/bin/env python3
"""Check CPA publication with real Git patches and temporary local fixtures."""

import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest


class MaterializationTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="cpa-materialization-test-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.source = self.root / "upstream"
        self.consumer = self.root / "consumer repo"
        self.source.mkdir()
        self.consumer.mkdir()
        self.output = self.consumer / "build/cliproxyapi"
        self.receipt = self.consumer / "build/cliproxyapi.receipt.json"
        self.real_git = shutil.which("git")
        self.environment = {
            key: value for key, value in os.environ.items()
            if not key.startswith("GIT_") and key not in {
                "GOWORK", "CLIPROXYAPI_BUILD_DIR", "INFRA_GO_DIR",
            }
        }
        self.environment.update(
            GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
            GIT_AUTHOR_NAME="Fixture", GIT_COMMITTER_NAME="Fixture",
            GIT_AUTHOR_EMAIL="fixture@example.invalid", GIT_COMMITTER_EMAIL="fixture@example.invalid",
        )
        self.git(self.source, "init", "-q", "--object-format=sha1")
        (self.source / "example.go").write_text("package example\nconst Value = 1\n")
        (self.source / "go.mod").write_text("module example.invalid/fixture\n\ngo 1.26.6\n")
        self.git(self.source, "add", ".")
        self.git(self.source, "commit", "-qm", "upstream")
        self.commit = self.git(self.source, "rev-parse", "HEAD")
        self.old_tree = self.git(self.source, "rev-parse", "HEAD^{tree}")
        (self.source / "example.go").write_text("package example\nconst Value = 2\n")
        self.git(self.source, "add", ".")
        self.git(self.source, "commit", "-qm", "fixture change")
        self.result_tree = self.git(self.source, "rev-parse", "HEAD^{tree}")
        patch = subprocess.check_output(
            [self.real_git, "-C", str(self.source), "format-patch", "-1", "--stdout"],
            env=self.environment,
        )
        bundle = "patches/cliproxyapi/feat/example"
        for directory in ("scripts", "docs/source-relationships", bundle,
                          "patches/cliproxyapi-sdk", "build/upstream", "build/cliproxyapi"):
            (self.consumer / directory).mkdir(parents=True, exist_ok=True)
        for name in ("materialize-cliproxyapi.sh", "check-patch-inventory.py"):
            shutil.copy2(Path(__file__).with_name(name), self.consumer / "scripts" / name)
        self.bundle = self.consumer / bundle
        (self.bundle / "example.go.patch").write_bytes(patch)
        (self.bundle / "apply_order.txt").write_text(
            "1 example.go.patch " + hashlib.sha256(patch).hexdigest() + "\n"
        )
        # No Go compilation or network: the real runner boundary verifies that
        # publication has not happened and that the stage has its own workspace.
        (self.bundle / "run.py").write_text("""import os
from pathlib import Path
stage = Path.cwd()
root = Path(os.environ['DOTAGENTS_REPO_ROOT'])
assert stage != root / 'build/cliproxyapi'
assert stage == Path(os.environ['DOTAGENTS_BUILD_PATH'])
assert stage == Path(os.environ['DOTAGENTS_SOURCE_PATH'])
assert Path(os.environ['GOWORK']) == stage / 'go.work'
assert str(Path(os.environ['INFRA_GO_DIR']).resolve()) in (stage / 'go.work').read_text()
assert stage.stat().st_dev == (root / 'build').stat().st_dev
assert 'Value = 2' in (stage / 'example.go').read_text()
""")
        relationship = {
            "schema_version": "git-source-relationship/v4", "kind": "patched_upstream_build",
            "integration": "build", "path": "build/cliproxyapi", "build_path": "build/cliproxyapi",
            "upstream": {"url": str(self.source), "commit": self.commit},
            "result_tree": self.result_tree, "patch_bundles": [bundle],
        }
        (self.consumer / "docs/source-relationships/cliproxyapi-materialization.json").write_text(
            json.dumps(relationship)
        )
        manifest = {
            "source_relationship": "docs/source-relationships/cliproxyapi-materialization.json",
            "output": "build/cliproxyapi", "upstream": {**relationship["upstream"], "tree": self.old_tree},
            "result_tree": self.result_tree,
        }
        (self.consumer / "patches/cliproxyapi/manifest.json").write_text(json.dumps(manifest))
        self.git(self.consumer, "clone", "--bare", "-q", str(self.source),
                 str(self.consumer / "build/upstream/cliproxyapi.git"))
        (self.output / "example.go").write_text("package example\nconst Value = 1\n")
        shutil.copy2(self.source / "go.mod", self.output / "go.mod")
        self.old_receipt = json.dumps({"result_tree": self.old_tree, "source_commit": self.commit}) + "\n"
        self.receipt.write_text(self.old_receipt)
        infra = self.root / "infra/go"
        for name in ("cache/disk", "cache/writeback", "logging",
                     "observability/diagnosticoutput", "observability/artifactcapture"):
            module = infra / name
            module.mkdir(parents=True)
            (module / "go.mod").write_text("module example.invalid/" + name + "\n")
        self.environment.update(
            INFRA_GO_DIR=str(infra), FIXTURE_OUTPUT=str(self.output),
            FIXTURE_MARKER=str(self.root / "injected"),
            FIXTURE_FOREIGN_GIT=str(self.source / ".git"),
            REAL_GIT=self.real_git, REAL_MV=shutil.which("mv"), REAL_RSYNC=shutil.which("rsync"),
        )
        wrappers = self.root / "bin"
        wrappers.mkdir()
        self.write_wrapper(wrappers / "git", """for arg in "$@"; do
  if [[ "$arg" == --index ]]; then
    if [[ "${FIXTURE_INJECT:-}" == after_check_edit ]]; then
      printf 'package example\\nconst Value = 99 // concurrent edit\\n' > "$FIXTURE_OUTPUT/example.go"
      touch "$FIXTURE_MARKER"
    elif [[ "${FIXTURE_INJECT:-}" == after_check_git ]]; then
      mkdir "$FIXTURE_OUTPUT/.git"
      touch "$FIXTURE_MARKER"
    fi
    break
  fi
done
exec "$REAL_GIT" "$@"
""")
        self.write_wrapper(wrappers / "mv", """if [[ "$1" == "$FIXTURE_OUTPUT" && "$2" == *cliproxyapi-recovery.*/source ]]; then
  if [[ "${FIXTURE_INJECT:-}" == before_rename_edit ]]; then
    printf 'package example\\nconst Value = 99 // concurrent edit\\n' > "$FIXTURE_OUTPUT/example.go"
    touch "$FIXTURE_MARKER"
  elif [[ "${FIXTURE_INJECT:-}" == before_rename_git ]]; then
    mkdir "$FIXTURE_OUTPUT/.git"
    touch "$FIXTURE_MARKER"
  fi
fi
exec "$REAL_MV" "$@"
""")
        self.write_wrapper(wrappers / "rsync", """if [[ "${FIXTURE_INJECT:-}" == during_copy_git ]]; then
  for arg in "$@"; do
    if [[ "$arg" == "$FIXTURE_OUTPUT/" && ! -e "$FIXTURE_MARKER" ]]; then
      ln -s "$FIXTURE_FOREIGN_GIT" "$FIXTURE_OUTPUT/.git"
      touch "$FIXTURE_MARKER"
      break
    fi
  done
fi
exec "$REAL_RSYNC" "$@"
""")
        self.environment["PATH"] = str(wrappers) + os.pathsep + self.environment["PATH"]

    def write_wrapper(self, path, body):
        path.write_text("#!/usr/bin/env bash\nset -euo pipefail\n" + body)
        path.chmod(0o755)

    def git(self, directory, *args):
        return subprocess.check_output(
            [self.real_git, "-C", str(directory), *args], env=self.environment,
            text=True, stderr=subprocess.PIPE,
        ).strip()

    def command(self):
        return ["bash", str(self.consumer / "scripts/materialize-cliproxyapi.sh")]

    def materialize(self, *, injection="", success=True):
        result = subprocess.run(
            self.command(), env={**self.environment, "FIXTURE_INJECT": injection},
            text=True, capture_output=True, timeout=30,
        )
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.consumer / "build/.cliproxyapi-materialization.lock").exists())
        self.assertEqual(list((self.consumer / "build").glob(".cliproxyapi-stage.*")), [])
        return result

    def assert_old_build(self):
        self.assertIn("Value = 1", (self.output / "example.go").read_text())
        self.assertEqual(self.receipt.read_text(), self.old_receipt)

    def tree_of(self, source):
        with tempfile.TemporaryDirectory(dir=self.root) as folder:
            copy = Path(folder) / "copy"
            shutil.copytree(source, copy, ignore=shutil.ignore_patterns("go.work", "go.work.sum"))
            self.git(copy, "init", "-q", "--object-format=sha1")
            self.git(copy, "add", "--force", "--all")
            return self.git(copy, "write-tree")

    def test_normal_publish_has_exact_tree_and_keeps_old_source(self):
        result = self.materialize()
        self.assertEqual(self.tree_of(self.output), self.result_tree)
        self.assertFalse((self.output / ".git").exists())
        self.assertEqual(json.loads(self.receipt.read_text())["result_tree"], self.result_tree)
        recoveries = list((self.consumer / "build").glob("cliproxyapi-recovery.*"))
        self.assertEqual(len(recoveries), 1)
        self.assertIn("recovery_snapshot=" + str(recoveries[0]), result.stdout)
        self.assertEqual(self.tree_of(recoveries[0] / "source"), self.old_tree)
        self.assertEqual((recoveries[0] / "receipt.json").read_text(), self.old_receipt)

    def test_first_build_publishes_without_a_recovery_copy(self):
        shutil.rmtree(self.output)
        self.receipt.unlink()
        self.materialize()
        self.assertEqual(self.tree_of(self.output), self.result_tree)
        self.assertEqual(json.loads(self.receipt.read_text())["result_tree"], self.result_tree)
        self.assertEqual(list((self.consumer / "build").glob("cliproxyapi-recovery.*")), [])

    def test_edits_after_initial_check_are_preserved(self):
        result = self.materialize(injection="after_check_edit", success=False)
        self.assertTrue((self.root / "injected").exists())
        self.assertIn("local changes", result.stderr)
        self.assertIn("Value = 99", (self.output / "example.go").read_text())
        self.assertEqual(self.receipt.read_text(), self.old_receipt)

    def test_foreign_git_inserted_after_initial_check_is_preserved(self):
        result = self.materialize(injection="after_check_git", success=False)
        self.assertTrue((self.output / ".git").is_dir())
        self.assertIn("foreign build path", result.stderr)
        self.assert_old_build()

    def test_foreign_git_inserted_during_snapshot_copy_is_not_initialized(self):
        config = (self.source / ".git/config").read_bytes()
        result = self.materialize(injection="during_copy_git", success=False)
        self.assertIn("foreign Git entry appeared", result.stderr)
        self.assertTrue((self.output / ".git").is_symlink())
        self.assertEqual((self.source / ".git/config").read_bytes(), config)
        self.assert_old_build()

    def test_edit_between_final_check_and_rename_is_restored(self):
        result = self.materialize(injection="before_rename_edit", success=False)
        self.assertTrue((self.root / "injected").exists())
        self.assertIn("restored previous", result.stderr)
        self.assertIn("Value = 99", (self.output / "example.go").read_text())
        self.assertEqual(self.receipt.read_text(), self.old_receipt)

    def test_foreign_git_between_final_check_and_rename_is_restored(self):
        result = self.materialize(injection="before_rename_git", success=False)
        self.assertTrue((self.output / ".git").is_dir())
        self.assertIn("restored previous", result.stderr)
        self.assert_old_build()

    def test_runner_failure_keeps_old_build_and_receipt(self):
        (self.bundle / "run.py").write_text("raise SystemExit('fixture runner failed')\n")
        result = self.materialize(success=False)
        self.assertIn("fixture runner failed", result.stderr)
        self.assert_old_build()
        self.assertEqual(list((self.consumer / "build").glob("cliproxyapi-recovery.*")), [])

    def test_runner_source_mutation_cannot_publish(self):
        (self.bundle / "run.py").write_text("from pathlib import Path\nPath('example.go').write_text('bad')\n")
        self.materialize(success=False)
        self.assert_old_build()

    def test_open_old_file_remains_recoverable_after_successful_publish(self):
        with (self.output / "example.go").open("r+") as held:
            self.materialize()
            held.seek(0)
            held.write("package example\nconst Value = 99 // late write\n")
            held.truncate()
        recovery = next((self.consumer / "build").glob("cliproxyapi-recovery.*"))
        self.assertIn("Value = 99", (recovery / "source/example.go").read_text())
        self.assertEqual(self.tree_of(self.output), self.result_tree)

    def test_materializers_use_exclusive_build_lock(self):
        ready, release = self.root / "ready", self.root / "release"
        (self.bundle / "run.py").write_text(
            "from pathlib import Path\nimport time\n"
            + "Path(" + repr(str(ready)) + ").touch()\n"
            + "deadline = time.monotonic() + 15\n"
            + "while not Path(" + repr(str(release)) + ").exists():\n"
            + "    if time.monotonic() > deadline: raise SystemExit('fixture timed out')\n"
            + "    time.sleep(.01)\n"
        )
        first = subprocess.Popen(
            self.command(), env=self.environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=True, start_new_session=True,
        )
        try:
            deadline = time.monotonic() + 10
            while not ready.exists() and first.poll() is None and time.monotonic() < deadline:
                time.sleep(.01)
            self.assertTrue(ready.exists(), "first materializer never reached its runner")
            self.assert_old_build()
            second = subprocess.run(self.command(), env=self.environment, text=True,
                                    capture_output=True, timeout=5)
            self.assertNotEqual(second.returncode, 0)
            self.assertIn("holds the build lock", second.stderr)
            self.assertTrue((self.consumer / "build/.cliproxyapi-materialization.lock").is_dir())
            self.assert_old_build()
            release.touch()
            stdout, stderr = first.communicate(timeout=20)
            self.assertEqual(first.returncode, 0, stdout + stderr)
            self.assertFalse((self.consumer / "build/.cliproxyapi-materialization.lock").exists())
            self.assertEqual(self.tree_of(self.output), self.result_tree)
        finally:
            if first.poll() is None:
                os.killpg(first.pid, signal.SIGTERM)
                first.communicate(timeout=5)


if __name__ == "__main__":
    unittest.main()
