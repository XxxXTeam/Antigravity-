package server

import (
	"testing"

	"github.com/antigravity/api-proxy/internal/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestTransformRequest_Basic(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
	}

	req := &models.ChatCompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []models.ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: 0.7,
	}

	googleReq := s.transformRequest(req)

	assert.Equal(t, "gemini-2.0-flash", googleReq.Model)
	assert.NotEmpty(t, googleReq.RequestID)
	assert.NotEmpty(t, googleReq.Request.SessionID)
	assert.Equal(t, 1, len(googleReq.Request.Contents))
	assert.Equal(t, "user", googleReq.Request.Contents[0].Role)
	assert.Equal(t, "Hello", googleReq.Request.Contents[0].Parts[0].Text)
	assert.Equal(t, 0.7, *googleReq.Request.GenerationConfig.Temperature)
}

func TestTransformRequest_ThinkingModel(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
	}

	req := &models.ChatCompletionRequest{
		Model: "gemini-2.0-flash-thinking",
		Messages: []models.ChatCompletionMessage{
			{Role: "user", Content: "Solve this"},
		},
	}

	googleReq := s.transformRequest(req)

	assert.Equal(t, "gemini-2.0-flash", googleReq.Model) // Suffix removed
	assert.NotNil(t, googleReq.Request.GenerationConfig.ThinkingConfig)
	assert.True(t, googleReq.Request.GenerationConfig.ThinkingConfig.IncludeThoughts)
}

func TestTransformRequest_SystemMessage(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
	}

	req := &models.ChatCompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []models.ChatCompletionMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hi"},
		},
	}

	googleReq := s.transformRequest(req)

	assert.NotNil(t, googleReq.Request.SystemInstruction)
	assert.Equal(t, "Be helpful", googleReq.Request.SystemInstruction.Parts[0].Text)
	assert.Equal(t, 1, len(googleReq.Request.Contents)) // Only user message in contents
}

func TestTransformRequest_Tools(t *testing.T) {
	s := &Server{
		logger: zap.NewNop(),
	}

	req := &models.ChatCompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []models.ChatCompletionMessage{
			{Role: "user", Content: "What time is it?"},
		},
		Tools: []models.Tool{
			{
				Type: "function",
				Function: models.Function{
					Name: "get_time",
				},
			},
		},
	}

	googleReq := s.transformRequest(req)

	assert.NotEmpty(t, googleReq.Request.Tools)
	assert.Equal(t, "get_time", googleReq.Request.Tools[0].FunctionDeclarations[0].Name)
}
