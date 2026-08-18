package guardrail

import (
	"encoding/json"
)

type ContentType string

const (
	ContentTypeSystem     ContentType = "system"
	ContentTypeUser       ContentType = "user"
	ContentTypeAssistant  ContentType = "assistant"
	ContentTypeToolCall   ContentType = "tool_call"
	ContentTypeToolResult ContentType = "tool_result"
)

type Content struct {
	Type   ContentType `json:"type"`
	Text   string      `json:"text"`
	Role   string      `json:"role,omitempty"`
	Source string      `json:"source,omitempty"`
}

func ExtractContent(body []byte) ([]Content, error) {
	if !json.Valid(body) {
		return []Content{{Type: ContentTypeUser, Text: string(body)}}, nil
	}

	var raw struct {
		Messages []struct {
			Role       string `json:"role"`
			Content    any    `json:"content"`
			ToolCalls  []struct {
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || len(raw.Messages) == 0 {
		return []Content{{Type: ContentTypeUser, Text: string(body)}}, nil
	}

	var contents []Content
	for _, msg := range raw.Messages {
		var contentStr string
		switch c := msg.Content.(type) {
		case string:
			contentStr = c
		case []any:
			for _, part := range c {
				if obj, ok := part.(map[string]any); ok {
					if text, ok := obj["text"].(string); ok {
						contentStr += text
					}
				}
			}
		default:
			contentStr = string(body)
		}

		if contentStr == "" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				contents = append(contents, Content{
					Type:   ContentTypeToolCall,
					Text:   tc.Function.Arguments,
					Role:   msg.Role,
					Source: tc.Function.Name,
				})
			}
			continue
		}

		if contentStr == "" {
			continue
		}

		var ct ContentType
		switch msg.Role {
		case "system":
			ct = ContentTypeSystem
		case "user":
			ct = ContentTypeUser
		case "assistant":
			ct = ContentTypeAssistant
		default:
			ct = ContentTypeUser
		}
		contents = append(contents, Content{
			Type: ct,
			Text: contentStr,
			Role: msg.Role,
		})
	}

	return contents, nil
}

func (c Content) String() string {
	return string(c.Type) + ": " + c.Text
}
