// Package xray provides AWS X-Ray integration for dindenault.
// Import this package only when you need X-Ray tracing functionality.
//
// X-Ray active tracing must be enabled on the Lambda function for trace data
// to be sent. When disabled, all calls are no-ops so there is no runtime cost.
package xray

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"github.com/navigacontentlab/dindenault"
	"github.com/navigacontentlab/dindenault/navigaid"
)

// Provider implements dindenault.TelemetryProvider with AWS X-Ray.
type Provider struct{}

// New creates a new X-Ray provider.
func New() *Provider {
	return &Provider{}
}

// Initialize implements dindenault.TelemetryProvider.
// Lambda's runtime configures the X-Ray daemon automatically, so no explicit
// initialization is required here.
func (p *Provider) Initialize(_ context.Context, _ string, _ dindenault.TelemetryOptions) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

// Interceptor implements dindenault.TelemetryProvider.
// Creates an X-Ray subsegment per RPC call and annotates it with the
// procedure name and (optionally) the caller's organization.
//
//nolint:ireturn // Returning interface as intended by TelemetryProvider design
func (p *Provider) Interceptor(logger *slog.Logger, opts dindenault.TelemetryOptions) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure

			ctx, seg := awsxray.BeginSubsegment(ctx, procedure)

			if err := awsxray.AddAnnotation(ctx, "procedure", procedure); err != nil {
				logger.Debug("X-Ray annotation failed", "error", err)
			}

			if opts.OrganizationFn != nil {
				if org := opts.OrganizationFn(ctx); org != "" {
					if err := awsxray.AddAnnotation(ctx, "organization", org); err != nil {
						logger.Debug("X-Ray annotation failed", "error", err)
					}
				}
			}

			resp, err := next(ctx, req)
			seg.Close(err)
			return resp, err
		}
	})
}

// InstrumentHandler implements dindenault.TelemetryProvider.
// Lambda's runtime sets up the X-Ray root segment automatically when active
// tracing is enabled on the function, so no additional wrapping is needed.
func (p *Provider) InstrumentHandler(handler interface{}) interface{} {
	return handler
}

// DefaultOrganizationFunction extracts the organization from Naviga ID auth claims.
// Use this as OrganizationFn in dindenault.TelemetryOptions.
func DefaultOrganizationFunction(ctx context.Context) string {
	info, err := navigaid.GetAuth(ctx)
	if err != nil {
		return "unknown"
	}

	return info.Claims.Org
}
