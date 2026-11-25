package mcp_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/navigacontentlab/dindenault/mcp"
)

const contentCall = `{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": {"name": "t", "arguments": {}}}`

func callResult(t *testing.T, tool mcp.Tool) map[string]any {
	t.Helper()

	tool.Name = "t"
	rr := post(t, mcp.NewServer("s", "1", tool), contentCall)
	require.Equal(t, http.StatusOK, rr.Code)

	resp := decodeResponse(t, rr.Body)
	require.Nil(t, resp["error"])

	return resp["result"].(map[string]any)
}

func TestToolsCall_ContentHandlerReturnsTextAndImage(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0}

	result := callResult(t, mcp.Tool{
		ContentHandler: func(_ context.Context, _ json.RawMessage) ([]mcp.Content, error) {
			return []mcp.Content{mcp.TextContent(`{"uuid":"a"}`), mcp.ImageContent(jpeg, "image/jpeg")}, nil
		},
	})

	assert.NotEqual(t, true, result["isError"])

	content := result["content"].([]any)
	require.Len(t, content, 2)
	assert.Equal(t, map[string]any{"type": "text", "text": `{"uuid":"a"}`}, content[0])
	assert.Equal(t, map[string]any{
		"type":     "image",
		"data":     base64.StdEncoding.EncodeToString(jpeg),
		"mimeType": "image/jpeg",
	}, content[1])
}

func TestToolsCall_ContentHandlerWinsOverHandler(t *testing.T) {
	result := callResult(t, mcp.Tool{
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`"handler"`), nil
		},
		ContentHandler: func(_ context.Context, _ json.RawMessage) ([]mcp.Content, error) {
			return []mcp.Content{mcp.TextContent("content handler")}, nil
		},
	})

	content := result["content"].([]any)
	require.Len(t, content, 1)
	assert.Equal(t, "content handler", content[0].(map[string]any)["text"])
}

func TestToolsCall_EmptyTextKeepsTheTextField(t *testing.T) {
	result := callResult(t, mcp.Tool{
		ContentHandler: func(_ context.Context, _ json.RawMessage) ([]mcp.Content, error) {
			return []mcp.Content{mcp.TextContent("")}, nil
		},
	})

	assert.Equal(t, map[string]any{"type": "text", "text": ""}, result["content"].([]any)[0])
}

func TestToolsCall_ContentHandlerErrors(t *testing.T) {
	for name, tool := range map[string]mcp.Tool{
		"handler error": {ContentHandler: func(_ context.Context, _ json.RawMessage) ([]mcp.Content, error) {
			return nil, errors.New("no thumbnail")
		}},
		"zero block": {ContentHandler: func(_ context.Context, _ json.RawMessage) ([]mcp.Content, error) {
			return []mcp.Content{{}}, nil
		}},
		"no handler": {},
	} {
		t.Run(name, func(t *testing.T) {
			result := callResult(t, tool)
			assert.Equal(t, true, result["isError"])
		})
	}
}

func TestContentMarshalRejectsZeroBlock(t *testing.T) {
	_, err := json.Marshal(mcp.Content{})
	assert.Error(t, err)
}
