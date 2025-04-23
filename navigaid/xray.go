package navigaid

import (
	"context"

	"github.com/aws/aws-xray-sdk-go/xray"
)

// AnnotationFunc is used to add authentication annotations to the context.
type AnnotationFunc func(ctx context.Context, organisation string, user string)

// XRayAnnotator implements the AnnotationFunc type.
func XRayAnnotator(ctx context.Context, organisation string, user string) {
	// Add user and organization as annotations to XRay segment
	seg := xray.GetSegment(ctx)
	if seg != nil {
		_ = seg.AddAnnotation("user", user)
		_ = seg.AddAnnotation("imid_org", organisation)
	}
}

// StartAuthSubsegment begins a new XRay subsegment for authentication.
func StartAuthSubsegment(ctx context.Context) (context.Context, *xray.Segment) {
	return xray.BeginSubsegment(ctx, "Authentication")
}

// This is an internal implementation of AddAnnotation for XRay.
func addXRayAnnotation(ctx context.Context, key string, value string) {
	seg := xray.GetSegment(ctx)
	if seg != nil {
		_ = seg.AddAnnotation(key, value)
	}
}
