// Package xray provides X-Ray tracing interceptors for Connect RPC services.
package xray

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/aws/aws-xray-sdk-go/xray"
)

// ExtractServiceAndMethod extracts the service name and method name from a Connect RPC procedure path.
// Connect procedure paths are typically in the form "/package.Service/Method".
func ExtractServiceAndMethod(procedure string) (string, string) {
	// Default values in case we can't extract them
	service, method := "unknown", "unknown"

	// A Connect procedure path is typically in the form "/package.Service/Method"
	parts := strings.Split(procedure, "/")

	if len(parts) >= 3 {
		// Extract service name (might include package prefix)
		serviceWithPackage := parts[1]
		serviceParts := strings.Split(serviceWithPackage, ".")

		if len(serviceParts) > 0 {
			service = serviceParts[len(serviceParts)-1]
		}

		// Extract method name
		method = parts[2]
	}

	return service, method
}

// Interceptor creates a Connect interceptor that adds AWS X-Ray tracing.
//
//nolint:ireturn
func Interceptor(name string) connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract procedure information
			procedure := req.Spec().Procedure
			service, method := ExtractServiceAndMethod(procedure)

			// Create a subsegment for this RPC call
			subCtx, seg := xray.BeginSubsegment(ctx, name+":"+service+"."+method)
			defer seg.Close(nil)

			// Add procedure information as annotations
			// Ignore errors as we can't do anything if annotation fails
			_ = seg.AddAnnotation("rpc.service", service)
			_ = seg.AddAnnotation("rpc.method", method)
			_ = seg.AddAnnotation("rpc.procedure", procedure)

			// Call the next handler with the X-Ray context
			resp, err := next(subCtx, req)

			// If there was an error, record it
			if err != nil {
				_ = seg.AddError(err)
			}

			return resp, err
		}
	})
}
