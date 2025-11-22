package models

// OpenAI API request/response models

// ChatRequest represents an OpenAI chat completion request
type ChatRequest struct {
	Model             string                 `json:"model" binding:"required"`
	Messages          []Message              `json:"messages" binding:"required"`
	Temperature       *float64               `json:"temperature,omitempty"`
	TopP              *float64               `json:"top_p,omitempty"`
	TopK              *int                   `json:"top_k,omitempty"`
	MaxTokens         *int                   `json:"max_tokens,omitempty"`
	Stream            bool                   `json:"stream,omitempty"`
	SystemInstruction string                 `json:"system_instruction,omitempty"`
	ResponseFormat    map[string]interface{} `json:"response_format,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ChatResponse represents an OpenAI chat completion response
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a single completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}


// ModelsResponse represents the OpenAI models list response
type ModelsResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// ModelObject represents a single model in the list
type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail provides error details
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}
