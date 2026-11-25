package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// Content types of a tools/call result, as in the MCP spec.
const (
	contentText  = "text"
	contentImage = "image"
)

// ContentHandler is a tool handler that returns the result's content blocks
// itself, for results that are more than one JSON value: text and images, for
// example. Build the blocks with TextContent and ImageContent.
//
// Every block goes back in one response. Behind an ALB a Lambda response may
// be at most 1 MB, and base64 makes an image a third larger, so send
// thumbnails rather than originals.
type ContentHandler func(ctx context.Context, input json.RawMessage) ([]Content, error)

// Content is one content block of a tools/call result.
type Content struct {
	typ      string
	text     string
	data     string
	mimeType string
}

// TextContent returns a text block.
func TextContent(text string) Content {
	return Content{typ: contentText, text: text}
}

// ImageContent returns an image block of data, which it base64-encodes.
// mimeType is the image's type, e.g. "image/jpeg". Clients that show images
// from tools display it, and the model sees it.
func ImageContent(data []byte, mimeType string) Content {
	return Content{typ: contentImage, data: base64.StdEncoding.EncodeToString(data), mimeType: mimeType}
}

// MarshalJSON writes the block in the shape its type has in the spec: text
// always with "text", an image with "data" and "mimeType".
func (c Content) MarshalJSON() ([]byte, error) {
	switch c.typ {
	case contentImage:
		//nolint:wrapcheck // marshals a plain struct
		return json.Marshal(struct {
			Type     string `json:"type"`
			Data     string `json:"data"`
			MimeType string `json:"mimeType"`
		}{c.typ, c.data, c.mimeType})
	case contentText:
		//nolint:wrapcheck // marshals a plain struct
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{c.typ, c.text})
	default:
		return nil, errors.New("mcp: content block without a type; use TextContent or ImageContent")
	}
}

// size is the block's payload length, for the audit event.
func (c Content) size() int {
	return len(c.text) + len(c.data)
}

// errNoHandler is a tool's error when it has neither Handler nor
// ContentHandler.
var errNoHandler = errors.New("tool has no handler")

// call runs the tool's handler and returns its content blocks. A Handler's
// JSON becomes one text block, as it always has.
func (t *Tool) call(ctx context.Context, input json.RawMessage) ([]Content, error) {
	switch {
	case t.ContentHandler != nil:
		content, err := t.ContentHandler(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, c := range content {
			if c.typ == "" {
				return nil, errors.New("mcp: content block without a type; use TextContent or ImageContent")
			}
		}

		return content, nil
	case t.Handler != nil:
		output, err := t.Handler(ctx, input)
		if err != nil {
			return nil, err
		}

		return []Content{TextContent(string(output))}, nil
	default:
		return nil, errNoHandler
	}
}
