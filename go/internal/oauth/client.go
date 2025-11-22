package oauth

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/antigravity/api-proxy/internal/models"
	"github.com/antigravity/api-proxy/internal/storage"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

// 内置的 OAuth 配置（不暴露在配置文件中）
const (
	oauthClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	oauthClientSecret = "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"
)

var oauthScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// 使用Google OAuth2 v2 endpoint
var googleOAuth2Endpoint = oauth2.Endpoint{
	AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL: "https://oauth2.googleapis.com/token",
}

// Client handles OAuth operations
type Client struct {
	config       *oauth2.Config
	logger       *zap.Logger
	server       *http.Server
	accountStore *storage.AccountStore
}

// NewClient creates a new OAuth client
func NewClient(callbackPort int, accountsDir string, logger *zap.Logger) *Client {
	// 构建回调URL - 使用当前服务器端口
	redirectURL := fmt.Sprintf("http://localhost:%d/oauth-callback", callbackPort)

	return &Client{
		config: &oauth2.Config{
			ClientID:     oauthClientID,
			ClientSecret: oauthClientSecret,
			RedirectURL:  redirectURL,
			Scopes:       oauthScopes,
			Endpoint:     googleOAuth2Endpoint, // 使用v2 endpoint
		},
		logger:       logger,
		accountStore: storage.NewAccountStore(accountsDir),
	}
}

// GetAuthCodeURL 生成OAuth授权URL（公开方法供外部调用）
func (c *Client) GetAuthCodeURL(state string) string {
	// 使用oauth2库生成标准的授权URL，包含所有必需参数
	return c.config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.ApprovalForce,
	)
}

// GetOAuthConfig 获取OAuth配置（用于token交换）
func (c *Client) GetOAuthConfig() *oauth2.Config {
	return c.config
}

// GetUserInfo 获取用户信息（公开方法）
func (c *Client) GetUserInfo(accessToken string) (*UserInfo, error) {
	return c.getUserInfo(accessToken)
}

// SaveAccountFromToken 从token和用户信息保存账号
func (c *Client) SaveAccountFromToken(token *oauth2.Token, userInfo *UserInfo) (*models.Account, error) {
	// 获取模型列表
	modelList, err := c.fetchModels(token.AccessToken)
	if err != nil {
		c.logger.Warn("Failed to fetch models", zap.Error(err))
		modelList = make(map[string]models.Model)
	}

	// 创建账号对象
	account := &models.Account{
		AccountID:     generateAccountID(userInfo.Email),
		Email:         userInfo.Email,
		Name:          userInfo.Name,
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		ExpiresIn:     int(time.Until(token.Expiry).Seconds()),
		Timestamp:     time.Now().UnixMilli(),
		Enable:        true,
		Models:        modelList,
		LastRefresh:   time.Now().UnixMilli(),
		RefreshStatus: "success",
		Usage: &models.UsageStats{
			TotalTokens:  0,
			InputTokens:  0,
			OutputTokens: 0,
			RequestCount: 0,
		},
		ErrorTracking: &models.ErrorTracking{
			ConsecutiveFailures: 0,
		},
	}

	// 保存账号
	if err := c.accountStore.Save(account); err != nil {
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	c.logger.Info("Account saved successfully",
		zap.String("email", account.Email),
		zap.String("account_id", account.AccountID),
		zap.Int("models", len(account.Models)))

	return account, nil
}

// StartLoginFlow starts the OAuth login flow and waits for callback
func (c *Client) StartLoginFlow() (*models.Account, error) {
	state := generateState()
	authURL := c.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	fmt.Println("\n🔐 Please open this URL in your browser to authorize:")
	fmt.Printf("\n%s\n\n", authURL)

	// 启动临时HTTP服务器接收回调
	resultChan := make(chan *models.Account, 1)
	errorChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth-callback", func(w http.ResponseWriter, r *http.Request) {
		account, err := c.handleCallback(w, r, state)
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- account
	})

	c.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", 8888),
		Handler: mux,
	}

	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("OAuth server error", zap.Error(err))
		}
	}()

	c.logger.Info("OAuth callback server started", zap.String("addr", c.server.Addr))

	// 等待结果或超时
	select {
	case account := <-resultChan:
		c.shutdown()
		return account, nil
	case err := <-errorChan:
		c.shutdown()
		return nil, err
	case <-time.After(5 * time.Minute):
		c.shutdown()
		return nil, fmt.Errorf("OAuth login timeout")
	}
}

func (c *Client) handleCallback(w http.ResponseWriter, r *http.Request, expectedState string) (*models.Account, error) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if state != expectedState {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return nil, fmt.Errorf("invalid state parameter")
	}

	if code == "" {
		http.Error(w, "No code provided", http.StatusBadRequest)
		return nil, fmt.Errorf("no authorization code")
	}

	// 交换token
	ctx := context.Background()
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	// 获取用户信息
	userInfo, err := c.getUserInfo(token.AccessToken)
	if err != nil {
		http.Error(w, "Failed to get user info", http.StatusInternalServerError)
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// 获取模型列表
	modelList, err := c.fetchModels(token.AccessToken)
	if err != nil {
		c.logger.Warn("Failed to fetch models", zap.Error(err))
		modelList = make(map[string]models.Model) // 继续，使用空模型列表
	}

	// 创建账号对象
	account := &models.Account{
		AccountID:     generateAccountID(userInfo.Email),
		Email:         userInfo.Email,
		Name:          userInfo.Name,
		AccessToken:   token.AccessToken,
		RefreshToken:  token.RefreshToken,
		ExpiresIn:     int(time.Until(token.Expiry).Seconds()),
		Timestamp:     time.Now().UnixMilli(),
		Enable:        true,
		Models:        modelList,
		LastRefresh:   time.Now().UnixMilli(),
		RefreshStatus: "success",
		Usage: &models.UsageStats{
			TotalTokens:  0,
			InputTokens:  0,
			OutputTokens: 0,
			RequestCount: 0,
		},
		ErrorTracking: &models.ErrorTracking{
			ConsecutiveFailures: 0,
		},
	}

	// 保存账号到文件
	if err := c.accountStore.Save(account); err != nil {
		c.logger.Error("Failed to save account", zap.Error(err))
		http.Error(w, "Failed to save account", http.StatusInternalServerError)
		return nil, fmt.Errorf("failed to save account: %w", err)
	}

	c.logger.Info("Account saved successfully",
		zap.String("account_id", account.AccountID),
		zap.String("email", account.Email))

	// 返回成功页面
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
		<html>
		<head><title>Login Successful</title></head>
		<body style="font-family: Arial, sans-serif; padding: 50px; text-align: center;">
			<h1 style="color: #27ae60;">✅ Login Successful!</h1>
			<p>Email: <strong>%s</strong></p>
			<p>Account ID: <code>%s</code></p>
			<p>Models: <strong>%d</strong></p>
			<hr>
			<p style="color: #7f8c8d;">You can close this window and return to the terminal.</p>
		</body>
		</html>
	`, account.Email, account.AccountID, len(modelList))

	return account, nil
}

func (c *Client) getUserInfo(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %s", string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	if userInfo.Name == "" {
		userInfo.Name = userInfo.Email
	}

	return &userInfo, nil
}

func (c *Client) fetchModels(accessToken string) (map[string]models.Model, error) {
	reqBody := []byte("{}")
	req, err := http.NewRequest("POST", "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:fetchAvailableModels", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Host", "daily-cloudcode-pa.sandbox.googleapis.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.3 antigravity/1.11.3")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Warn("Failed to fetch models - non-200 response",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return make(map[string]models.Model), nil // 返回空列表继续
	}

	// 处理 gzip 压缩的响应
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
		c.logger.Debug("Response is gzip compressed, decompressing...")
	}

	// 读取完整响应用于调试
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("Models API response",
		zap.Int("body_length", len(bodyBytes)),
		zap.String("body_preview", string(bodyBytes[:min(200, len(bodyBytes))])))

	var result struct {
		Models map[string]interface{} `json:"models"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		c.logger.Warn("Failed to decode models response",
			zap.Error(err),
			zap.String("body", string(bodyBytes)))
		return make(map[string]models.Model), nil // 继续，返回空列表
	}

	if result.Models == nil {
		c.logger.Warn("Models API returned null models field")
		return make(map[string]models.Model), nil
	}

	modelList := make(map[string]models.Model)
	for modelID := range result.Models {
		modelList[modelID] = models.Model{
			ID:      modelID,
			Object:  "model",
			OwnedBy: "google",
		}
	}

	c.logger.Info("Fetched models successfully",
		zap.Int("count", len(modelList)),
		zap.Strings("model_ids", getModelIDs(modelList)))
	return modelList, nil
}

// 辅助函数：获取模型ID列表（用于日志）
func getModelIDs(models map[string]models.Model) []string {
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	return ids
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) shutdown() {
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.server.Shutdown(ctx)
	}
}

// UserInfo represents Google user information
type UserInfo struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func generateAccountID(email string) string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%s_%x", email, b)
}
