# AWS X-Ray Integration for Dindenault

The `xray` package provides optional AWS X-Ray tracing for dindenault services.

## Prerequisites

Active tracing must be enabled on the Lambda function (via `TracingConfig: Active` in SAM/CDK/Terraform or the AWS console). When disabled, all SDK calls are no-ops with no runtime cost.

## Installation

```bash
go get github.com/navigacontentlab/dindenault/xray@latest
```

## Usage

```go
package main

import (
    "log/slog"

    "github.com/aws/aws-lambda-go/lambda"
    "github.com/navigacontentlab/dindenault"
    xrayprovider "github.com/navigacontentlab/dindenault/xray"
)

func main() {
    logger := slog.Default()

    provider := xrayprovider.New()

    telemetryOpts := dindenault.TelemetryOptions{
        OrganizationFn: xrayprovider.DefaultOrganizationFunction,
    }

    app := dindenault.New(logger,
        dindenault.WithService(path, handler),
        dindenault.WithInterceptors(
            dindenault.LoggingInterceptors(logger),
            dindenault.TelemetryInterceptor(logger, provider, telemetryOpts),
        ),
    )

    lambda.Start(app.Handle())
}
```

## What gets traced

Each Connect RPC call gets its own X-Ray subsegment named after the procedure (e.g. `mypackage.v1.MyService/MyMethod`). The subsegment carries two annotations:

| Annotation     | Value                                      |
|----------------|--------------------------------------------|
| `procedure`    | Full Connect procedure path                |
| `organization` | Caller's org from Naviga ID (if available) |

Errors returned from handlers are automatically recorded on the subsegment.

## How it works

Lambda's runtime creates the root X-Ray segment when active tracing is enabled. The `xray` provider only adds subsegments under that root — no daemon configuration or explicit initialization is required.
