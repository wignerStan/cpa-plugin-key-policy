# Homebrew formulas

The formulas live with the plugin source so the plugin, patched host, patch
manifest, and release workflow have one maintenance owner. The patched host is
generated from `vendor/` at build time; Homebrew never uses a nested fork
checkout.

For a private or rolling install, tap this repository explicitly and use the
HEAD formulas:

```bash
brew tap wignerStan/cpa-plugin-key-policy \
  https://github.com/wignerStan/cpa-plugin-key-policy.git
brew install --HEAD wignerStan/cpa-plugin-key-policy/cpa-key-policy
brew install --HEAD wignerStan/cpa-plugin-key-policy/cliproxyapi-patched
```

The formulas intentionally track `main`. Tagged releases are also published by
the repository release workflow as platform archives and checksums.
