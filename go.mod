module github.com/oshokin/alarm-button

go 1.27.1

require (
	github.com/doitdistributed/go-update v0.0.0-20210408142833-fae09717712d
	github.com/mitchellh/go-ps v1.0.0
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.12.1
	go.uber.org/zap v1.28.0
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/mod v0.41.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/telemetry v0.0.0-20260908163034-4bcc4b2ee518 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
	golang.org/x/vuln v1.8.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2 // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	golang.org/x/vuln/cmd/govulncheck
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
