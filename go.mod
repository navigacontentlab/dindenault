module github.com/navigacontentlab/dindenault

go 1.26.0

toolchain go1.26.11

// v1.10.0 added per-tool OAuth scopes, which were removed again.
retract v1.10.0

require (
	connectrpc.com/connect v1.21.1
	github.com/aws/aws-lambda-go v1.55.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/stretchr/testify v1.12.1
	google.golang.org/protobuf v1.36.12
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect
