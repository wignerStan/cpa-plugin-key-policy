# Infra OS file subscriptions

The plugin directly imports the public module
`github.com/wignerStan/infra/go/files/notify`. The implementation belongs to
`https://github.com/wignerStan/infra`. This repository contains no copy of it.
The module path and canonical infra commit are recorded in
`infra-file-notifications.json`.

The infra module restores the original implementation from infra commit
`c1be68e8f818b649e4b66411db76cb8819f6f79b`. Its focused dependency graph includes
fsnotify without importing the infra root module.

The kit watches the parent directory and filters events for the selected file.
This preserves subscriptions after atomic replacement. `Read` blocks on OS
notifications and supports context cancellation; `Close` releases the OS watch.
The plugin adapts these notifications to its policy cache. It does not poll.

The plugin retains policy-specific validation, content hashing, error reporting,
and cache updates. Policy reload reads JSON while holding the policy writer lock
so an old snapshot cannot overwrite a later committed policy. Usage-only writes
do not invalidate policy. There is no `poll_interval` worker setting.

For local builds, the module defaults to `$HOME/infra/go/files/notify`.
Set `INFRA_GO_DIR` to select another infra Go checkout. The build helper writes
an ignored root `go.work` with a direct module replacement. `go.mod` also has a
relative replacement for the standard sibling-checkout layout.
The helper uses the existing checkout without creating a worktree, copying
source or generating module manifests. Existing user-managed Go workspaces
are left intact.

```sh
make test INFRA_GO_DIR=/path/to/infra/go
```

CI checks out the recorded commit at `build/infra-source`, then binds its public
module directly through `INFRA_GO_DIR=./build/infra-source/go`. The recorded commit
must be available in the infra remote before CI or release builds can fetch it.
Changes to the adapter must be made and committed in infra. Then update the
dependency record and run the plugin notification tests against that commit.
