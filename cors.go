package dindenault

import (
	"net/http"
	"strings"

	"github.com/rs/cors"
)

// DefaultCORSDomains returns the default allowed domain suffixes.
func DefaultCORSDomains() []string {
	return []string{".infomaker.io", ".navigacloud.com"}
}

// CORSOptions controls the behaviour of the CORS middleware.
type CORSOptions struct {
	// AllowHTTP determines if HTTP (non-HTTPS) origins are allowed
	AllowHTTP bool

	// AllowedDomains is a list of domain suffixes that are allowed in CORS requests
	// e.g. [".navigaglobal.com", ".infomaker.io"]
	AllowedDomains []string

	// Custom allows overriding the default CORS options with custom settings
	Custom cors.Options
}

// DefaultCorsMiddleware creates a middleware with the default
// settings.
func DefaultCORSMiddleware() *cors.Cors {
	return NewCORSMiddleware(CORSOptions{})
}

// NewCORSMiddleware creates a CORS middleware suitable for our
// editorial application APIs.
func NewCORSMiddleware(opts CORSOptions) *cors.Cors {
	if len(opts.AllowedDomains) == 0 {
		opts.AllowedDomains = DefaultCORSDomains()
	}

	coreOpts := opts.Custom

	if len(coreOpts.AllowedMethods) == 0 {
		coreOpts.AllowedMethods = []string{http.MethodPost}
	}

	allowFn := standardAllowOriginFunc(
		opts.AllowHTTP, opts.AllowedDomains,
	)

	if coreOpts.AllowOriginFunc != nil {
		allowFn = anyOfAllowOriginFuncs(coreOpts.AllowOriginFunc, allowFn)
	}

	coreOpts.AllowOriginFunc = allowFn

	return cors.New(coreOpts)
}

// standardAllowOriginFunc creates a function that validates CORS origins
// based on the configured allowed domains and HTTP settings.
func standardAllowOriginFunc(
	allowHTTP bool, allowedDomains []string,
) func(origin string) bool {
	return func(origin string) bool {
		// Reject non-HTTPS origins if HTTP is not allowed
		if !allowHTTP && !strings.HasPrefix(origin, "https://") {
			return false
		}

		// Check if origin ends with any of the allowed domain suffixes
		for _, domain := range allowedDomains {
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}

		return false
	}
}

// anyOfAllowOriginFuncs combines multiple origin validator functions
// and returns true if any of them approves the origin.
func anyOfAllowOriginFuncs(funcs ...func(string) bool) func(string) bool {
	return func(origin string) bool {
		for _, validatorFn := range funcs {
			if validatorFn(origin) {
				return true
			}
		}

		return false
	}
}
