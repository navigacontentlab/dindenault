module github.com/navigacontentlab/dindenault/xray

go 1.25

require (
	connectrpc.com/connect v1.19.1
	github.com/aws/aws-xray-sdk-go v1.8.5
	github.com/navigacontentlab/dindenault v1.0.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/aws/aws-lambda-go v1.48.0 // indirect
	github.com/aws/aws-sdk-go v1.47.9 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.52.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/grpc v1.64.1 // indirect
	google.golang.org/protobuf v1.36.9 // indirect
)

// Use local version of dindenault during development
replace github.com/navigacontentlab/dindenault => ../
