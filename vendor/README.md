# Generated vendor materializations

`vendor/` is not an editable source tree. The generated CLIProxyAPI checkout is
materialized at `vendor/cliproxyapi/` by:

```bash
bash scripts/materialize-cliproxyapi.sh
```

The output is intentionally ignored by `vendor/.gitignore`. Its source
authority is the upstream commit and ordered patch set recorded in
`patches/cliproxyapi/manifest.json`; it never contains a nested `.git`
directory or an independently maintained fork.
