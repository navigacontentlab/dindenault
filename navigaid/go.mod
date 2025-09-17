module github.com/navigacontentlab/dindenault/navigaid

go 1.23.6

require (
	connectrpc.com/connect v1.18.1
	github.com/golang-jwt/jwt/v4 v4.5.2
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/trace v1.35.0
)

require (
	golang.org/x/net v0.39.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/navigacontentlab/dindenault => ../
