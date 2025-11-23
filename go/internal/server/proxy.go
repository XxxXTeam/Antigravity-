
package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/antigravity/api-proxy/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	googleAPIURL = "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:streamGenerateContent?alt=sse"
	googleHost   = "daily-cloudcode-pa.sandbox.googleapis.com"
	userAgent    = "antigravity/1.11.3 windows/amd64"
)

// chatCompletions handles the chat completion request
func (s *Server) chatCompletions(c *gin.Context) {
	var req models.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	const maxRetries = 5
	var lastErr error

	// Retry loop for handling transient errors and account rotation
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get a valid token
		account, err := s.oauthClient.GetToken()
		if err != nil {
			s.logger.Error("Failed to get token",
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second) // Brief backoff
			continue
		}

		s.logger.Info("Using account for request",
			zap.String("account_id", account.AccountID),
			zap.String("email", account.Email),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", maxRetries))

		// Transform request to Google format
		googleReq := s.transformRequest(&req)

		// Prepare HTTP request
		reqBody, err := json.Marshal(googleReq)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to marshal request"})
			return
		}

		// Debug log
		s.logger.Debug("Sending request to Google",
			zap.String("account_id", account.AccountID),
			zap.String("email", account.Email),
			zap.Int("body_length", len(reqBody)))

		httpReq, err := http.NewRequest("POST", googleAPIURL, bytes.NewReader(reqBody))
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to create request"})
			return
		}

		httpReq.Header.Set("Host", googleHost)
		httpReq.Header.Set("User-Agent", userAgent)
		httpReq.Header.Set("Authorization", "Bearer "+account.AccessToken)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept-Encoding", "gzip")

		// Send request
		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			s.logger.Warn("Google API request failed",
				zap.String("account_id", account.AccountID),
				zap.String("email", account.Email),
				zap.Int("attempt", attempt+1),
				zap.Error(err))
			account.RecordFailure(err.Error())
			s.oauthClient.AccountStore().Save(account)
			lastErr = err
			continue // Retry with next account
		}
		defer resp.Body.Close()

		// Handle non-200 responses
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)

			// Special handling for 429 Rate Limit
			if resp.StatusCode == 429 {
				s.logger.Warn("Rate limit encountered",
					zap.String("account_id", account.AccountID),
					zap.String("email", account.Email),
					zap.Int("attempt", attempt+1),
					zap.Int("rate_limit_count", account.ErrorTracking.RateLimitCount+1))
				account.RecordRateLimit()
				s.oauthClient.AccountStore().Save(account)
				lastErr = fmt.Errorf("rate limit exceeded")
				continue // Try next account immediately
			}

			// Special handling for 403 Permission Denied
			if resp.StatusCode == 403 {
				s.logger.Warn("Permission denied - disabling account",
					zap.String("account_id", account.AccountID),
					zap.String("email", account.Email),
					zap.String("error", string(body)))
				account.RecordPermissionDenied()
				s.oauthClient.AccountStore().Save(account)
				lastErr = fmt.Errorf("permission denied")
				continue // Try next account immediately
			}

			// Other errors
			s.logger.Warn("Google API returned error",
				zap.String("account_id", account.AccountID),
				zap.String("email", account.Email),
				zap.Int("status", resp.StatusCode),
				zap.String("body", string(body)),
				zap.Int("attempt", attempt+1))

			account.RecordFailure(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			s.oauthClient.AccountStore().Save(account)

			// For 4xx errors (other than 429), don't retry
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				c.JSON(resp.StatusCode, gin.H{"error": "Upstream API error", "details": string(body)})
				return
			}

			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			continue // Retry for 5xx errors
		}

		// Success! Record and process response
		s.logger.Info("Request successful",
			zap.String("account_id", account.AccountID),
			zap.String("email", account.Email),
			zap.Int("attempt", attempt+1))

		account.RecordSuccess()
		s.oauthClient.AccountStore().Save(account)

		// Handle streaming response
		if req.Stream {
			s.handleStreamResponse(c, resp.Body, req.Model, account)
			return
		}

		// Handle normal response (aggregate SSE)
		s.handleNormalResponse(c, resp.Body, req.Model, account)
		return
	}

	// All retries exhausted
	s.logger.Error("All retry attempts exhausted",
		zap.Int("attempts", maxRetries),
		zap.Error(lastErr))
	c.JSON(503, gin.H{
		"error":   "Service unavailable after retries",
		"details": lastErr.Error(),
		"retries": maxRetries,
	})
}

func (s *Server) transformRequest(req *models.ChatCompletionRequest) *models.GoogleRequest {
	// Determine model name and thinking config
	modelName := req.Model
	enableThinking := strings.HasSuffix(modelName, "-thinking") || 
		modelName == "gemini-2.5-pro" || 
		strings.HasPrefix(modelName, "gemini-3-pro-")
	
	if strings.HasSuffix(modelName, "-thinking") {
		modelName = strings.TrimSuffix(modelName, "-thinking")
	}

	// Build contents
	var contents []models.GoogleContent
	var systemInstruction *models.GoogleSystemInstruction

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			// Handle system message
			text := ""
			if str, ok := msg.Content.(string); ok {
				text = str
			}
			systemInstruction = &models.GoogleSystemInstruction{
				Role: "user", // Google system instruction uses 'user' role internally sometimes, or specific field
				Parts: []models.GooglePart{{Text: text}},
			}
			continue
		}

		parts := []models.GooglePart{}
		
		// Handle content (string or array)
		switch v := msg.Content.(type) {
		case string:
			parts = append(parts, models.GooglePart{Text: v})
		case []interface{}:
			for _, item := range v {
				if partMap, ok := item.(map[string]interface{}); ok {
					if partMap["type"] == "text" {
						if text, ok := partMap["text"].(string); ok {
							parts = append(parts, models.GooglePart{Text: text})
						}
					} else if partMap["type"] == "image_url" {
						// Handle image (simplified for now, assumes base64 in url)
						if imgURL, ok := partMap["image_url"].(map[string]interface{}); ok {
							if url, ok := imgURL["url"].(string); ok {
								// Extract base64
								if strings.HasPrefix(url, "data:image/") {
									partsStr := strings.Split(url, ";base64,")
									if len(partsStr) == 2 {
										mimeType := strings.TrimPrefix(partsStr[0], "data:")
										parts = append(parts, models.GooglePart{
											InlineData: &models.GoogleInlineData{
												MimeType: mimeType,
												Data:     partsStr[1],
											},
										})
									}
								}
							}
						}
					}
				}
			}
		}

		// Handle tool calls from previous turn (if any)
		// Note: In OpenAI, tool calls are in the message. In Google, they are parts.
		// This implementation assumes standard user/assistant flow for now.

		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		contents = append(contents, models.GoogleContent{
			Role:  role,
			Parts: parts,
		})
	}

	// Build generation config
	genConfig := models.GoogleGenerationConfig{
		CandidateCount: 1,
		StopSequences: []string{
			"<|user|>", "<|bot|>", "<|context_request|>", "<|endoftext|>", "<|end_of_turn|>",
		},
	}

	if req.Temperature != 0 {
		genConfig.Temperature = &req.Temperature
	}
	if req.TopP != 0 {
		genConfig.TopP = &req.TopP
	}
	if req.TopK != 0 {
		genConfig.TopK = &req.TopK
	}
	if req.MaxTokens != 0 {
		genConfig.MaxOutputTokens = &req.MaxTokens
	}

	if enableThinking {
		budget := 8192
		genConfig.ThinkingConfig = &models.GoogleThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  budget,
		}
		
		// Ensure MaxOutputTokens is greater than ThinkingBudget
		// If user didn't set it, or set it too low, we override it
		minMaxTokens := budget + 4096 // Buffer for actual response
		if genConfig.MaxOutputTokens == nil || *genConfig.MaxOutputTokens <= budget {
			genConfig.MaxOutputTokens = &minMaxTokens
		}
	}

	// Log the generation config for debugging
	if enableThinking {
		configBytes, _ := json.Marshal(genConfig)
		fmt.Printf("DEBUG: Generation Config: %s\n", string(configBytes))
	}

	// Build tools
	var googleTools []models.GoogleTool
	if len(req.Tools) > 0 {
		funcs := []models.GoogleFunctionDeclaration{}
		for _, t := range req.Tools {
			if t.Type == "function" {
				funcs = append(funcs, models.GoogleFunctionDeclaration{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				})
			}
		}
		if len(funcs) > 0 {
			googleTools = append(googleTools, models.GoogleTool{
				FunctionDeclarations: funcs,
			})
		}
	}

	return &models.GoogleRequest{
		Project:   generateProjectID(),
		RequestID: "agent-" + uuid.New().String(),
		Model:     modelName,
		UserAgent: "antigravity",
		Request: models.GoogleInner{
			Contents:          contents,
			GenerationConfig:  genConfig,
			SessionID:         generateSessionID(),
			SystemInstruction: systemInstruction,
			Tools:             googleTools,
		},
	}
}

func (s *Server) handleNormalResponse(c *gin.Context, body io.Reader, model string, account *models.Account) {
	// Aggregate SSE response
	scanner := bufio.NewScanner(body)
	content := ""
	reasoning := ""
	var totalTokens, inputTokens, outputTokens int64
	
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var googleResp models.GoogleResponse
		if err := json.Unmarshal([]byte(dataStr), &googleResp); err != nil {
			continue
		}

		if len(googleResp.Response.Candidates) > 0 {
			candidate := googleResp.Response.Candidates[0]
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					if part.Thought {
						reasoning += part.Text
					} else {
						content += part.Text
					}
				}
			}
		}

		// Track usage metadata
		if googleResp.Response.UsageMetadata != nil {
			inputTokens = int64(googleResp.Response.UsageMetadata.PromptTokenCount)
			outputTokens = int64(googleResp.Response.UsageMetadata.CandidatesTokenCount)
			totalTokens = int64(googleResp.Response.UsageMetadata.TotalTokenCount)
		}
	}

	// Record usage in account
	if account.Usage != nil {
		account.Usage.TotalTokens += totalTokens
		account.Usage.InputTokens += inputTokens
		account.Usage.OutputTokens += outputTokens
		account.Usage.RequestCount++
		s.oauthClient.AccountStore().Save(account)
	}

	// Record usage in usage store
	if err := s.usageStore.RecordUsage(account.AccountID, inputTokens, outputTokens); err != nil {
		s.logger.Warn("Failed to record usage", zap.Error(err))
	}

	// Estimate tokens if not provided by API
	if totalTokens == 0 {
		// Rough estimate: ~4 chars per token
		totalTokens = int64(len(content) / 4)
		outputTokens = totalTokens
	}

	// Fallback: Extract thinking content if present (regex)
	if reasoning == "" {
		// Regex to match <think>...</think> content, allowing for newlines (using (?s))
		re := regexp.MustCompile(`(?s)<think>(.*?)</think>`)
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			reasoning = strings.TrimSpace(matches[1]) // Use captured group and trim whitespace
			
			// Remove the thinking part from content
			content = strings.Replace(content, matches[0], "", 1)
			content = strings.TrimSpace(content)
		}
	}

	resp := models.ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []models.ChatCompletionChoice{
			{
				Index: 0,
				Message: models.ChatCompletionMessage{
					Role:      "assistant",
					Content:   content,
					Reasoning: reasoning,
				},
				FinishReason: "stop",
			},
		},
		Usage: &models.Usage{
			PromptTokens:     int(inputTokens),
			CompletionTokens: int(outputTokens),
			TotalTokens:      int(totalTokens),
		},
	}

	c.JSON(200, resp)
}

func (s *Server) handleStreamResponse(c *gin.Context, body io.Reader, model string, account *models.Account) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var totalTokens, inputTokens, outputTokens int64

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		if dataStr == "[DONE]" {
			break
		}

		var googleResp models.GoogleResponse
		if err := json.Unmarshal([]byte(dataStr), &googleResp); err != nil {
			continue
		}

		if len(googleResp.Response.Candidates) == 0 {
			continue
		}

		// Track usage metadata
		if googleResp.Response.UsageMetadata != nil {
			inputTokens = int64(googleResp.Response.UsageMetadata.PromptTokenCount)
			outputTokens = int64(googleResp.Response.UsageMetadata.CandidatesTokenCount)
			totalTokens = int64(googleResp.Response.UsageMetadata.TotalTokenCount)
		}

		candidate := googleResp.Response.Candidates[0]
		
		for _, part := range candidate.Content.Parts {
			chunk := models.ChatCompletionChunk{
				ID:      "chatcmpl-" + uuid.New().String(),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []models.ChatCompletionChunkChoice{
					{
						Index: 0,
						Delta: models.ChatCompletionDelta{
							Content: part.Text,
						},
					},
				},
			}

			// Send chunk
			respBytes, _ := json.Marshal(chunk)
			c.Writer.Write([]byte("data: " + string(respBytes) + "\n\n"))
			c.Writer.Flush()
		}
	}

	// Record usage in account
	if account.Usage != nil {
		account.Usage.TotalTokens += totalTokens
		account.Usage.InputTokens += inputTokens
		account.Usage.OutputTokens += outputTokens
		account.Usage.RequestCount++
		s.oauthClient.AccountStore().Save(account)
	}

	// Record usage in usage store
	if err := s.usageStore.RecordUsage(account.AccountID, inputTokens, outputTokens); err != nil {
		s.logger.Warn("Failed to record usage", zap.Error(err))
	}

	c.Writer.Write([]byte("data: [DONE]\n\n"))
}

func generateProjectID() string {
	adjectives := []string{"useful", "bright", "swift", "calm", "bold"}
	nouns := []string{"fuze", "wave", "spark", "flow", "core"}
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	num := rand.Intn(100000)
	return fmt.Sprintf("%s-%s-%d", adj, noun, num)
}

func generateSessionID() string {
	return fmt.Sprintf("-%d", rand.Int63())
}
