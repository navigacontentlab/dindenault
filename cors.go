package didenault

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
	AllowHTTP      bool
	AllowedDomains []string
	Custom         cors.Options
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

func standardAllowOriginFunc(
	allowHTTP bool, allowedDomains []string,
) func(origin string) bool {
	return func(origin string) bool {
		if !allowHTTP && !strings.HasPrefix(origin, "https://") {
			return false
		}

		for _, domain := range allowedDomains {
			if strings.HasSuffix(origin, domain) {
				return true
			}
		}

		return false
	}
}

func anyOfAllowOriginFuncs(funcs ...func(string) bool) func(string) bool {
	return func(s string) bool {
		for _, fn := range funcs {
			if fn(s) {
				return true
			}
		}

		return false
	}
}
