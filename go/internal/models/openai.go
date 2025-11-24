package models

// OpenAI Chat Completion Request
type ChatCompletionRequest struct {
	Model            string                  `json:"model"`
	Messages         []ChatCompletionMessage `json:"messages"`
	Stream           bool                    `json:"stream,omitempty"`
	MaxTokens        int                     `json:"max_tokens,omitempty"`
	Temperature      float64                 `json:"temperature,omitempty"`
	TopP             float64                 `json:"top_p,omitempty"`
	TopK             int                     `json:"top_k,omitempty"` // Google specific
	Tools            []Tool                  `json:"tools,omitempty"`
	ToolChoice       interface{}             `json:"tool_choice,omitempty"`
	FrequencyPenalty float64                 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                 `json:"presence_penalty,omitempty"`
}

type ChatCompletionMessage struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or []ContentPart
	Reasoning  string      `json:"reasoning,omitempty"` // Custom field for thinking content
	Name       string      `json:"name,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type ToolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// OpenAI Chat Completion Response
type ChatCompletionResponse struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []ChatCompletionChoice `json:"choices"`
	Usage             *Usage                 `json:"usage,omitempty"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI Stream Response
type ChatCompletionChunk struct {
	ID                string                      `json:"id"`
	Object            string                      `json:"object"`
	Created           int64                       `json:"created"`
	Model             string                      `json:"model"`
	SystemFingerprint string                      `json:"system_fingerprint,omitempty"`
	Choices           []ChatCompletionChunkChoice `json:"choices"`
}

type ChatCompletionChunkChoice struct {
	Index        int         `json:"index"`
	Delta        ChatCompletionDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"` // Nullable
}

type ChatCompletionDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	Reasoning string     `json:"reasoning,omitempty"` // Custom field for thinking models
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Google Cloud Code API Request Structures (Internal)
type GoogleRequest struct {
	Project   string      `json:"project"`
	RequestID string      `json:"requestId"`
	Request   GoogleInner `json:"request"`
	Model     string      `json:"model"`
	UserAgent string      `json:"userAgent"`
}

type GoogleInner struct {
	Contents          []GoogleContent          `json:"contents"`
	GenerationConfig  GoogleGenerationConfig   `json:"generationConfig"`
	SessionID         string                   `json:"sessionId"`
	SystemInstruction *GoogleSystemInstruction `json:"systemInstruction,omitempty"`
	Tools             []GoogleTool             `json:"tools,omitempty"`
	ToolConfig        *GoogleToolConfig        `json:"toolConfig,omitempty"`
}

type GoogleContent struct {
	Role  string       `json:"role"`
	Parts []GooglePart `json:"parts"`
}

type GooglePart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *GoogleInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GoogleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GoogleFunctionResponse `json:"functionResponse,omitempty"`
	Thought          bool                    `json:"thought,omitempty"` // Check if this field exists
}

type GoogleInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GoogleFunctionCall struct {
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type GoogleFunctionResponse struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type GoogleGenerationConfig struct {
	TopP           *float64              `json:"topP,omitempty"`
	TopK           *int                  `json:"topK,omitempty"`
	Temperature    *float64              `json:"temperature,omitempty"`
	CandidateCount int                   `json:"candidateCount"`
	MaxOutputTokens *int                 `json:"maxOutputTokens,omitempty"`
	StopSequences  []string              `json:"stopSequences,omitempty"`
	ThinkingConfig *GoogleThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GoogleThinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts"`
	ThinkingBudget  *int   `json:"thinkingBudget,omitempty"`  // For Gemini 2.5 and earlier
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`   // For Gemini 3 and later
}

type GoogleSystemInstruction struct {
	Role  string       `json:"role"`
	Parts []GooglePart `json:"parts"`
}

type GoogleTool struct {
	FunctionDeclarations []GoogleFunctionDeclaration `json:"functionDeclarations"`
}

type GoogleFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type GoogleToolConfig struct {
	FunctionCallingConfig GoogleFunctionCallingConfig `json:"functionCallingConfig"`
}

type GoogleFunctionCallingConfig struct {
	Mode string `json:"mode"`
}

// Google API Response
type GoogleResponse struct {
	Response GoogleResponseInner `json:"response"`
}

type GoogleResponseInner struct {
	Candidates    []GoogleCandidate `json:"candidates"`
	UsageMetadata *GoogleUsage      `json:"usageMetadata,omitempty"`
}

type GoogleCandidate struct {
	Content      GoogleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type GoogleUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}
