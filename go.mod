module cpa-key-policy

go 1.23.0

require (
	github.com/wignerStan/infra/go/files/notify v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
)

replace github.com/wignerStan/infra/go/files/notify => ../../infra/go/files/notify
