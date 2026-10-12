# Infra OS file subscriptions

The plugin consumes the OS subscription kit maintained at
`infra/go/files/notify`. Its generated source is byte-identical to the committed
infra file pinned in `infra-file-notifications.json`. Maintain the implementation
in infra, not in this repository. The existing infra licenses are retained under
`third_party/infra`.

The kit watches the parent directory and filters events for the selected file.
This preserves subscriptions after atomic replacement. `Read` blocks on OS
notifications and supports context cancellation; `Close` releases the OS watch.
The infra `config/dynamic.SubscribeFile` adapter can connect this primitive to
an external-store cache subscription. Neither layer has a periodic query loop.

The plugin retains policy-specific validation, content hashing, error reporting,
and cache updates. Policy reload reads JSON while holding the policy writer lock
so an old snapshot cannot overwrite a later committed policy. Usage-only writes
do not invalidate policy. There is no `poll_interval` worker setting.

Refresh and verify the generated source with:

```sh
python3 scripts/vendor-infra-file-notifications.py --update-from /path/to/infra
python3 scripts/vendor-infra-file-notifications.py
```
