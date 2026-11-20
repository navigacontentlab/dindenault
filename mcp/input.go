package mcp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/navigacontentlab/dindenault/navigaid"
)

// Multi round-trip requests (2026-07-28, SEP-2322): instead of sending the
// client a request of its own, a server answers tools/call with an
// InputRequiredResult — resultType "input_required", the questions in
// inputRequests and an opaque requestState. The client asks the user and calls
// the tool again with the answers in params.inputResponses and the state
// echoed in params.requestState. Nothing is kept on the server between the two
// calls.
//
// A tool asks by returning RequireInput from its handler. On the call that
// comes back, InputFromContext gives it the answers and the state it sent. The
// server signs the state with the key given to WithInputRequired and refuses a
// retry whose state was changed, has expired, belongs to another caller or to
// another tool call.
//
// Only form-mode elicitation is offered, and only to a client that declared
// it in _meta["io.modelcontextprotocol/clientCapabilities"].elicitation on a
// 2026-07-28 request. For every other client Input.CanElicit is false and the
// tool should give the error it gives today.

// Elicitation actions a client answers with (ElicitResult.action).
const (
	ElicitAccept  = "accept"
	ElicitDecline = "decline"
	ElicitCancel  = "cancel"
)

// OutcomeInputRequired is the audit outcome of a tools/call answered with an
// InputRequiredResult: the tool asked the user and did nothing else yet.
const OutcomeInputRequired = "input_required"

const (
	resultInputRequired = "input_required"
	methodElicit        = "elicitation/create"
	elicitModeForm      = "form"

	// minStateKeyLen is the shortest HMAC key WithInputRequired accepts.
	minStateKeyLen = 32
	// DefaultRequestStateTTL is how long a requestState is accepted when
	// WithInputRequired is given no TTL.
	DefaultRequestStateTTL = 15 * time.Minute

	stateVersion = 1
	// stateMACContext separates these MACs from anything else signed with the
	// same key.
	stateMACContext = "dindenault/mcp/requestState/v1."
)

// ErrInputNotSupported is the error a tool call gets when its handler returns
// RequireInput for a client that cannot answer (see Input.CanElicit). Handlers
// should check CanElicit first and return their own error instead.
var ErrInputNotSupported = errors.New("this tool needs an answer from the user, and the client does not support elicitation")

// errRequestState is wrapped by every reason a requestState is refused.
var errRequestState = errors.New("requestState rejected")

// Elicitation is one form-mode elicitation/create request: a message for the
// user and the schema of the answer. RequestedSchema must be a JSON Schema
// object with flat primitive properties, as the spec restricts it (string,
// number, integer, boolean, and enums including multi-select arrays of
// strings).
type Elicitation struct {
	Message         string
	RequestedSchema json.RawMessage
}

// ElicitResult is the client's answer to one Elicitation.
type ElicitResult struct {
	// Action is ElicitAccept, ElicitDecline or ElicitCancel.
	Action string `json:"action"`
	// Content is the submitted form, only present when Action is accept.
	Content map[string]any `json:"content,omitempty"`
}

// Accepted reports whether the user submitted the form.
func (r ElicitResult) Accepted() bool { return r.Action == ElicitAccept }

// InputRequiredError is returned by a handler (through RequireInput) to answer
// the call with an InputRequiredResult instead of a result.
type InputRequiredError struct {
	// Elicitations are keyed by a name the handler chooses; the answers come
	// back under the same keys.
	Elicitations map[string]Elicitation
	// State is marshalled to JSON, signed and sent as requestState. The handler
	// gets it back from Input.State on the retry. It is readable by the
	// client (signed, not encrypted), so it should hold ids, not secrets.
	State any
}

func (e *InputRequiredError) Error() string {
	return fmt.Sprintf("input required (%d elicitations)", len(e.Elicitations))
}

// RequireInput returns the error a handler returns to ask the user. state may
// be nil.
func RequireInput(state any, elicitations map[string]Elicitation) error {
	return &InputRequiredError{Elicitations: elicitations, State: state}
}

// Input is what a tool handler knows about asking the user on this call.
type Input struct {
	canElicit bool
	retry     bool
	state     json.RawMessage
	responses map[string]json.RawMessage
}

type inputKey struct{}

// InputFromContext returns the Input of the current tools/call. Outside a
// tools/call, or for a client that cannot be asked, it is the zero Input:
// CanElicit and Retry are false.
func InputFromContext(ctx context.Context) Input {
	in, _ := ctx.Value(inputKey{}).(Input)

	return in
}

// CanElicit reports whether the handler may return RequireInput: the request
// is of the 2026-07-28 revision, the client declared form elicitation, and
// the server has a key to sign the state with (WithInputRequired).
func (in Input) CanElicit() bool { return in.canElicit }

// Retry reports whether this call carries a requestState the server signed
// for this caller, this tool and these arguments, within its TTL.
func (in Input) Retry() bool { return in.retry }

// State unmarshals the state the handler passed to RequireInput into v. It
// returns an error when the call is not a Retry.
func (in Input) State(v any) error {
	if !in.retry {
		return errors.New("no request state on this call")
	}

	if err := json.Unmarshal(in.state, v); err != nil {
		return fmt.Errorf("unmarshal request state: %w", err)
	}

	return nil
}

// Elicited returns the client's answer to the elicitation sent under key, and
// false when there is none or it cannot be read.
//
// The answers are what the client sends; unlike the requestState they are not
// signed. An answer shows what the caller chose, not that a person saw the
// question, so it is not a security confirmation on its own.
func (in Input) Elicited(key string) (ElicitResult, bool) {
	raw, ok := in.responses[key]
	if !ok {
		return ElicitResult{}, false
	}

	var r ElicitResult
	if err := json.Unmarshal(raw, &r); err != nil || r.Action == "" {
		return ElicitResult{}, false
	}

	return r, true
}

// stateSigner signs and verifies requestState.
type stateSigner struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// WithInputRequired lets tools ask the user (RequireInput) on 2026-07-28
// requests from clients that declared elicitation. key signs the requestState
// with HMAC-SHA256 and must be at least 32 random bytes, kept secret and shared
// by every instance of the service (a Lambda's concurrent copies included).
// ttl bounds how long the user may take to answer; zero means
// DefaultRequestStateTTL. It returns s for chaining and panics on a short
// key, as that is a deployment error.
//
// Without it no client is offered elicitation: Input.CanElicit is false, and a
// call that comes back with a requestState is refused.
func (s *Server) WithInputRequired(key []byte, ttl time.Duration) *Server {
	if len(key) < minStateKeyLen {
		panic(fmt.Sprintf("mcp: WithInputRequired needs a key of at least %d bytes", minStateKeyLen))
	}

	if ttl <= 0 {
		ttl = DefaultRequestStateTTL
	}

	s.signer = &stateSigner{key: bytes.Clone(key), ttl: ttl, now: time.Now}

	return s
}

// stateEnvelope is what requestState carries, before signing.
type stateEnvelope struct {
	V    int             `json:"v"`
	Tool string          `json:"tool"`
	Args string          `json:"args"` // SHA-256 of the canonical arguments
	Org  string          `json:"org,omitempty"`
	Sub  string          `json:"sub,omitempty"`
	Exp  int64           `json:"exp"`
	Data json.RawMessage `json:"data,omitempty"`
}

// principal returns the validated caller, empty for an unauthenticated server.
func principal(ctx context.Context) (string, string) {
	if auth, err := navigaid.GetAuth(ctx); err == nil {
		return auth.Claims.Org, auth.Claims.Subject
	}

	return "", ""
}

// argsDigest fingerprints the arguments independently of key order and
// whitespace, since a client may re-encode them on the retry.
func argsDigest(args json.RawMessage) string {
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return fingerprint(args)
	}

	canon, err := json.Marshal(v)
	if err != nil {
		return fingerprint(args)
	}

	sum := sha256.Sum256(canon)

	return hex.EncodeToString(sum[:])
}

func (g *stateSigner) mac(payload string) string {
	m := hmac.New(sha256.New, g.key)
	m.Write([]byte(stateMACContext + payload))

	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (g *stateSigner) sign(ctx context.Context, tool string, args json.RawMessage, data any) (string, error) {
	var raw json.RawMessage

	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return "", fmt.Errorf("marshal request state: %w", err)
		}

		raw = b
	}

	org, sub := principal(ctx)

	b, err := json.Marshal(stateEnvelope{
		V: stateVersion, Tool: tool, Args: argsDigest(args), Org: org, Sub: sub,
		Exp: g.now().Add(g.ttl).Unix(), Data: raw,
	})
	if err != nil {
		return "", fmt.Errorf("marshal request state: %w", err)
	}

	payload := base64.RawURLEncoding.EncodeToString(b)

	return payload + "." + g.mac(payload), nil
}

// verify checks a requestState against the call it came back on and returns
// the handler's data.
func (g *stateSigner) verify(ctx context.Context, token, tool string, args json.RawMessage) (json.RawMessage, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(g.mac(payload))) {
		return nil, fmt.Errorf("%w: bad signature", errRequestState)
	}

	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: bad encoding", errRequestState)
	}

	var env stateEnvelope
	if err := json.Unmarshal(b, &env); err != nil || env.V != stateVersion {
		return nil, fmt.Errorf("%w: bad payload", errRequestState)
	}

	org, sub := principal(ctx)

	switch {
	case g.now().Unix() > env.Exp:
		return nil, fmt.Errorf("%w: expired", errRequestState)
	case env.Org != org || env.Sub != sub:
		return nil, fmt.Errorf("%w: issued to another caller", errRequestState)
	case env.Tool != tool || env.Args != argsDigest(args):
		return nil, fmt.Errorf("%w: issued for another call", errRequestState)
	}

	return env.Data, nil
}

// clientCanElicitForm reports whether 2026-07-28 client capabilities declare
// form-mode elicitation: an elicitation object with "form", or with neither
// mode (form is implied), but not "url" alone.
func clientCanElicitForm(meta map[string]any) bool {
	caps, _ := meta[metaClientCapabilities].(map[string]any)

	el, ok := caps["elicitation"].(map[string]any)
	if !ok {
		return false
	}

	_, form := el["form"]
	_, url := el["url"]

	return form || !url
}

// wire types of the InputRequiredResult.

type elicitParams struct {
	Mode            string          `json:"mode"`
	Message         string          `json:"message"`
	RequestedSchema json.RawMessage `json:"requestedSchema"`
}

type inputRequest struct {
	Method string       `json:"method"`
	Params elicitParams `json:"params"`
}

type inputRequiredResult struct {
	ResultType    string                  `json:"resultType"`
	InputRequests map[string]inputRequest `json:"inputRequests"`
	RequestState  string                  `json:"requestState,omitempty"`
}

// inputRequiredResult builds the result for a handler's RequireInput.
func (s *Server) inputRequiredResult(
	ctx context.Context, tool string, args json.RawMessage, e *InputRequiredError,
) (inputRequiredResult, error) {
	if len(e.Elicitations) == 0 {
		return inputRequiredResult{}, errors.New("RequireInput needs at least one elicitation")
	}

	reqs := make(map[string]inputRequest, len(e.Elicitations))

	for key, el := range e.Elicitations {
		if key == "" || el.Message == "" || !isJSONObject(el.RequestedSchema) {
			return inputRequiredResult{}, fmt.Errorf("elicitation %q needs a key, a message and an object schema", key)
		}

		reqs[key] = inputRequest{
			Method: methodElicit,
			Params: elicitParams{Mode: elicitModeForm, Message: el.Message, RequestedSchema: el.RequestedSchema},
		}
	}

	state, err := s.signer.sign(ctx, tool, args, e.State)
	if err != nil {
		return inputRequiredResult{}, err
	}

	return inputRequiredResult{ResultType: resultInputRequired, InputRequests: reqs, RequestState: state}, nil
}
