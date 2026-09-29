package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// Correlation headers. An agent sets them on every MCP request so a tool call
// can be tied back to the conversation and the single prompt that caused it,
// and the server forwards them on its own downstream calls (see
// NewHTTPClient). All are optional.
const (
	// HeaderCorrelationID identifies the whole workflow, e.g. an agent chat.
	HeaderCorrelationID = "X-Correlation-Id"
	// HeaderTurnID identifies one step of it, e.g. a single user prompt.
	HeaderTurnID = "X-Turn-Id"
	// HeaderTraceParent is the W3C Trace Context header. When absent, a
	// tools/call's params._meta.traceparent is used instead.
	HeaderTraceParent = "traceparent"
)

// defaultServerName is the server name dindenault.WithMCP and WithMCPAuth use.
const defaultServerName = "dindenault"

// maxCorrelationLen caps each correlation value copied from a request, so a
// caller cannot inflate every audit line with an arbitrarily long header.
const maxCorrelationLen = 128

// maxAuditErrorLen caps the error text in an audit event. The full error still
// reaches the caller; the audit line only needs enough to classify it.
const maxAuditErrorLen = 300

// Correlation holds the correlation header values of the current request.
type Correlation struct {
	ID          string
	TurnID      string
	TraceParent string
}

const correlationKey contextKey = authorizationKey + 1

// CorrelationFromContext returns the correlation headers of the MCP request
// being handled. Values the caller did not send are empty.
func CorrelationFromContext(ctx context.Context) Correlation {
	c, _ := ctx.Value(correlationKey).(Correlation)

	return c
}

// Header returns the non-empty correlation values as HTTP headers, for
// forwarding on a downstream request.
func (c Correlation) Header() http.Header {
	h := http.Header{}

	for name, v := range map[string]string{
		HeaderCorrelationID: c.ID,
		HeaderTurnID:        c.TurnID,
		HeaderTraceParent:   c.TraceParent,
	} {
		if v != "" {
			h.Set(name, v)
		}
	}

	return h
}

func correlationFromRequest(r *http.Request) Correlation {
	return Correlation{
		ID:          clip(r.Header.Get(HeaderCorrelationID), maxCorrelationLen),
		TurnID:      clip(r.Header.Get(HeaderTurnID), maxCorrelationLen),
		TraceParent: clip(r.Header.Get(HeaderTraceParent), maxCorrelationLen),
	}
}

// withMetaTraceParent fills in TraceParent from a tools/call request's
// params._meta when the caller sent no traceparent header. MCP clients that
// propagate OpenTelemetry context the MCP way (e.g. Strands' MCPClient) put it
// there, per call, rather than in an HTTP header.
func withMetaTraceParent(ctx context.Context, meta map[string]any) context.Context {
	c := CorrelationFromContext(ctx)
	if c.TraceParent != "" {
		return ctx
	}

	tp, ok := meta[HeaderTraceParent].(string)
	if !ok || tp == "" {
		return ctx
	}

	c.TraceParent = clip(tp, maxCorrelationLen)

	return context.WithValue(ctx, correlationKey, c)
}

func withCorrelation(ctx context.Context, r *http.Request) context.Context {
	if _, ok := ctx.Value(correlationKey).(Correlation); ok {
		return ctx
	}

	return context.WithValue(ctx, correlationKey, correlationFromRequest(r))
}

// Audit outcomes.
const (
	OutcomeOK              = "ok"              // the tool ran and returned a result
	OutcomeToolError       = "tool_error"      // the tool ran and returned an error
	OutcomeDenied          = "denied"          // authenticated, but missing RequiredPermissions
	OutcomeUnauthenticated = "unauthenticated" // no or invalid token (AuthMiddleware)
	OutcomeUnknownTool     = "unknown_tool"    // no tool with that name
	OutcomeInvalidParams   = "invalid_params"  // tools/call params could not be decoded
)

// AuditEventName is the "event" value of every audit line, for log filters
// such as a CloudWatch subscription filter on { $.event = "mcp_tool_call" }.
const AuditEventName = "mcp_tool_call"

// AuditVersion is bumped when a field changes meaning or is removed. Adding a
// field does not bump it.
const AuditVersion = 1

// AuditEvent is one structured record per tools/call — who called which tool,
// as part of which workflow, and what came of it. Tool arguments and results
// are never recorded (they are customer content): the arguments are
// fingerprinted with SHA-256 so a call can be matched against a known input
// without storing it.
type AuditEvent struct {
	Event        string `json:"event"`
	AuditVersion int    `json:"audit_version"`
	Time         string `json:"time"`
	// Service is the server name given to NewServer, or the Lambda function
	// name when the server uses dindenault's default name.
	Service       string `json:"service"`
	Tool          string `json:"tool"`
	ReadOnly      bool   `json:"read_only"`
	Outcome       string `json:"outcome"`
	Error         string `json:"error,omitempty"`
	Org           string `json:"org,omitempty"`
	Subject       string `json:"sub,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	TurnID        string `json:"turn_id,omitempty"`
	TraceParent   string `json:"traceparent,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	ArgsSHA256    string `json:"args_sha256,omitempty"`
	ArgsBytes     int    `json:"args_bytes"`
	ResultBytes   int    `json:"result_bytes"`
}

// AuditSink receives one AuditEvent per tools/call. It must be safe for
// concurrent use and should not block: it runs on the request path.
type AuditSink func(ctx context.Context, event AuditEvent)

// NewJSONAuditSink returns a sink that writes each event as one JSON line to w.
//
// Audit lines are written independently of the service's log level: a
// service running at LOG_LEVEL=warn must still leave an audit trail.
func NewJSONAuditSink(w io.Writer) AuditSink {
	var mu sync.Mutex

	return func(_ context.Context, event AuditEvent) {
		line, err := json.Marshal(event)
		if err != nil {
			return
		}

		mu.Lock()
		defer mu.Unlock()

		_, _ = w.Write(append(line, '\n'))
	}
}

// DefaultAuditSink writes JSON lines to stdout — in Lambda, the function's
// CloudWatch log group.
//
//nolint:gochecknoglobals // shared default so every server writes through one mutex
var DefaultAuditSink = NewJSONAuditSink(os.Stdout)

// DisableAudit is an AuditSink that drops every event.
func DisableAudit(context.Context, AuditEvent) {}

func newAuditEvent(ctx context.Context, service, tool string) AuditEvent {
	c := CorrelationFromContext(ctx)

	event := AuditEvent{
		Event:         AuditEventName,
		AuditVersion:  AuditVersion,
		Time:          time.Now().UTC().Format(time.RFC3339Nano),
		Service:       service,
		Tool:          tool,
		CorrelationID: c.ID,
		TurnID:        c.TurnID,
		TraceParent:   c.TraceParent,
	}

	if auth, err := navigaid.GetAuth(ctx); err == nil {
		event.Org = auth.Claims.Org
		event.Subject = auth.Claims.Subject
	}

	return event
}

func fingerprint(args json.RawMessage) string {
	sum := sha256.Sum256(args)

	return hex.EncodeToString(sum[:])
}

func auditServiceName(name string) string {
	if name != "" && name != defaultServerName {
		return name
	}

	if fn := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); fn != "" {
		return fn
	}

	return name
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}

	return s
}
