package mcp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
)

// The 2026-07-28 revision of MCP is stateless: there is no initialize
// handshake and no session. Every request names its protocol version and the
// client's capabilities in params._meta, and repeats the version, method and
// (for tools/call) tool name in HTTP headers. A request carrying the version in
// _meta is handled by the rules in this file; everything else keeps the
// initialize-based rules of 2025-11-25 and earlier, so both kinds of client
// work against the same endpoint.

// modernProtocolVersions are the stateless revisions this server speaks,
// newest first.
var modernProtocolVersions = []string{"2026-07-28"} //nolint:gochecknoglobals // fixed table

// Request metadata keys and headers of the stateless revisions.
const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"

	headerProtocolVersion = "MCP-Protocol-Version"
	headerMethod          = "Mcp-Method"
	headerName            = "Mcp-Name"
)

// Error codes of the stateless revisions.
const (
	codeHeaderMismatch             = -32020
	codeUnsupportedProtocolVersion = -32022
)

// listTTLMillis is how long clients may cache tools/list and server/discover.
// The tool list of a Server is fixed for its lifetime, so the answer is the
// same for every caller ("public").
const (
	listTTLMillis    = 5 * 60 * 1000
	cacheScopePublic = "public"
	resultComplete   = "complete"
)

// requestMeta is the part of params a stateless request is recognised and
// validated by.
type requestMeta struct {
	Name string                     `json:"name"`
	Meta map[string]json.RawMessage `json:"_meta"`
}

// modernVersion returns the protocol version a stateless request names in
// params._meta, and whether it names one at all.
func modernVersion(params json.RawMessage) (string, requestMeta, bool) {
	var m requestMeta
	if len(params) == 0 || json.Unmarshal(params, &m) != nil {
		return "", m, false
	}

	raw, ok := m.Meta[metaProtocolVersion]
	if !ok {
		return "", m, false
	}

	var v string
	_ = json.Unmarshal(raw, &v)

	return v, m, true
}

// isModernHeader reports whether an MCP-Protocol-Version header names a
// stateless revision. Legacy clients send their negotiated (older) version in
// the same header, so only a version newer than every legacy one counts.
func isModernHeader(v string) bool {
	return protocolVersionRe.MatchString(v) && v > supportedProtocolVersions[0]
}

// allProtocolVersions lists every version this server speaks, newest first.
func allProtocolVersions() []string {
	return append(slices.Clone(modernProtocolVersions), supportedProtocolVersions...)
}

// decodeHeaderName undoes the base64 sentinel clients use for header-unsafe
// names: "=?base64?<value>?=".
func decodeHeaderName(v string) string {
	if inner, ok := strings.CutPrefix(v, "=?base64?"); ok {
		if inner, ok = strings.CutSuffix(inner, "?="); ok {
			if b, err := base64.StdEncoding.DecodeString(inner); err == nil {
				return string(b)
			}
		}
	}

	return v
}

// validateModern checks a stateless request's version, metadata and headers.
// It writes the error response and returns false when the request is invalid.
func validateModern(w http.ResponseWriter, r *http.Request, req *jsonRPCRequest, version string, m requestMeta) bool {
	if !slices.Contains(modernProtocolVersions, version) {
		writeErrorStatus(w, http.StatusBadRequest, req.ID, codeUnsupportedProtocolVersion, "Unsupported protocol version",
			map[string]any{"supported": allProtocolVersions(), "requested": version})

		return false
	}

	if caps, ok := m.Meta[metaClientCapabilities]; !ok || !isJSONObject(caps) {
		writeErrorStatus(w, http.StatusBadRequest, req.ID, codeInvalidParams,
			"Invalid params: _meta."+metaClientCapabilities+" must be an object", nil)

		return false
	}

	mismatch := func(header, got, want string) bool {
		if got == want {
			return false
		}

		writeErrorStatus(w, http.StatusBadRequest, req.ID, codeHeaderMismatch,
			"Header mismatch: "+header+" header value '"+got+"' does not match body value '"+want+"'", nil)

		return true
	}

	if mismatch(headerProtocolVersion, r.Header.Get(headerProtocolVersion), version) ||
		mismatch(headerMethod, r.Header.Get(headerMethod), req.Method) {
		return false
	}

	if req.Method == methodToolsCall && mismatch(headerName, decodeHeaderName(r.Header.Get(headerName)), m.Name) {
		return false
	}

	return true
}

func isJSONObject(raw json.RawMessage) bool {
	var v map[string]json.RawMessage

	return json.Unmarshal(raw, &v) == nil && v != nil
}

// discoverResult answers server/discover.
type discoverResult struct {
	ResultType        string             `json:"resultType"`
	SupportedVersions []string           `json:"supportedVersions"`
	Capabilities      serverCapabilities `json:"capabilities"`
	Meta              map[string]any     `json:"_meta"`
	TTLMillis         int                `json:"ttlMs"`
	CacheScope        string             `json:"cacheScope"`
}

func (s *Server) handleDiscover(w http.ResponseWriter, req *jsonRPCRequest) {
	writeResult(w, req.ID, discoverResult{
		ResultType:        resultComplete,
		SupportedVersions: modernProtocolVersions,
		Capabilities:      serverCapabilities{Tools: &toolsCapability{}},
		Meta:              map[string]any{metaServerInfo: s.info()},
		TTLMillis:         listTTLMillis,
		CacheScope:        cacheScopePublic,
	})
}
