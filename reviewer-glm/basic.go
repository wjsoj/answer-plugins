/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package basic

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/apache/answer-plugins/reviewer-glm/i18n"
	"github.com/apache/answer-plugins/util"
	"github.com/apache/answer/plugin"
	myI18n "github.com/segmentfault/pacman/i18n"
	"github.com/segmentfault/pacman/log"
)

//go:embed  info.yaml
var Info embed.FS

type rateLimiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
}

func newRateLimiter(maxTokens int, refillRate time.Duration) *rateLimiter {
	return &rateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)

	if tokensToAdd > 0 {
		rl.tokens = min(rl.tokens+tokensToAdd, rl.maxTokens)
		rl.lastRefill = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) updateConfig(maxTokens int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if maxTokens > 0 {
		rl.maxTokens = maxTokens
		// Adjust current tokens if needed
		if rl.tokens > maxTokens {
			rl.tokens = maxTokens
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var globalRateLimiter = newRateLimiter(10, 1*time.Second) // 10 requests per second

type reviewMetrics struct {
	mu                sync.RWMutex
	totalReviews      int64
	approvedReviews   int64
	rejectedReviews   int64
	cacheHits         int64
	apiErrors         int64
	rateLimitExceeded int64
}

var globalMetrics = &reviewMetrics{}

func (m *reviewMetrics) recordReview(approved bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalReviews++
	if approved {
		m.approvedReviews++
	} else {
		m.rejectedReviews++
	}
}

func (m *reviewMetrics) recordCacheHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheHits++
}

func (m *reviewMetrics) recordAPIError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.apiErrors++
}

func (m *reviewMetrics) recordRateLimitExceeded() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimitExceeded++
}

func (m *reviewMetrics) getStats() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]int64{
		"total_reviews":       m.totalReviews,
		"approved_reviews":    m.approvedReviews,
		"rejected_reviews":    m.rejectedReviews,
		"cache_hits":          m.cacheHits,
		"api_errors":          m.apiErrors,
		"rate_limit_exceeded": m.rateLimitExceeded,
	}
}

type cacheEntry struct {
	approved  bool
	timestamp time.Time
}

type reviewCache struct {
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	reviewer *Reviewer
}

var globalCache = &reviewCache{
	entries: make(map[string]cacheEntry),
}

func (c *reviewCache) get(key string, cacheTTL time.Duration) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return false, false
	}

	if time.Since(entry.timestamp) > cacheTTL {
		return false, false
	}

	return entry.approved, true
}

func (c *reviewCache) set(key string, approved bool, maxSize int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		approved:  approved,
		timestamp: time.Now(),
	}

	// Clean old entries if cache is too large
	if len(c.entries) > maxSize {
		c.cleanup(maxSize)
	}
}

func (c *reviewCache) cleanup(maxSize int) {
	// Remove oldest entries if cache exceeds max size
	if len(c.entries) <= maxSize {
		return
	}

	// Simple cleanup: remove entries until we're under the limit
	toRemove := len(c.entries) - maxSize
	for key := range c.entries {
		if toRemove <= 0 {
			break
		}
		delete(c.entries, key)
		toRemove--
	}
}

type Reviewer struct {
	Config *ReviewerConfig
}

type ReviewerConfig struct {
	APIKey           string `json:"api_key"`
	ReviewQuestion   bool   `json:"review_question"`
	ReviewAnswer     bool   `json:"review_answer"`
	ReviewComment    bool   `json:"review_comment"`
	SpamFiltering    string `json:"spam_filtering"`
	APITimeout       int    `json:"api_timeout"`        // Timeout in seconds
	MaxContentLength int    `json:"max_content_length"` // Maximum content length in characters
	CacheTTL         int    `json:"cache_ttl"`          // Cache TTL in minutes
	CacheMaxSize     int    `json:"cache_max_size"`     // Maximum cache entries
	RateLimitRPS     int    `json:"rate_limit_rps"`     // Rate limit requests per second
	MaxRetries       int    `json:"max_retries"`        // Maximum retry attempts
}

type GLMRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
}

type GLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
	ContentFilter []struct {
		Level int    `json:"level"`
		Role  string `json:"role"`
	} `json:"contentFilter,omitempty"`
}

// ContentFilterError indicates that GLM detected unsafe content in the input
type ContentFilterError struct {
	Message string
	Code    string
}

func (e *ContentFilterError) Error() string {
	return fmt.Sprintf("content filter triggered (code: %s): %s", e.Code, e.Message)
}

func init() {
	plugin.Register(&Reviewer{
		Config: &ReviewerConfig{
			ReviewQuestion:   true,
			ReviewAnswer:     true,
			ReviewComment:    true,
			SpamFiltering:    "review",
			APITimeout:       30,
			MaxContentLength: 8000,
			CacheTTL:         60,   // 60 minutes
			CacheMaxSize:     1000, // 1000 entries
			RateLimitRPS:     10,   // 10 requests per second
			MaxRetries:       2,    // 2 retry attempts
		},
	})

	// Start periodic metrics logging
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			stats := globalMetrics.getStats()
			log.Infof("GLM Reviewer Metrics: total=%d, approved=%d, rejected=%d, cache_hits=%d, api_errors=%d, rate_limit_exceeded=%d",
				stats["total_reviews"], stats["approved_reviews"], stats["rejected_reviews"],
				stats["cache_hits"], stats["api_errors"], stats["rate_limit_exceeded"])
		}
	}()
}

func (r *Reviewer) Info() plugin.Info {
	info := &util.Info{}
	info.GetInfo(Info)

	return plugin.Info{
		Name:        plugin.MakeTranslator(i18n.InfoName),
		SlugName:    info.SlugName,
		Description: plugin.MakeTranslator(i18n.InfoDescription),
		Author:      info.Author,
		Version:     info.Version,
		Link:        info.Link,
	}
}
func (r *Reviewer) Review(content *plugin.ReviewContent) (result *plugin.ReviewResult) {
	result = &plugin.ReviewResult{Approved: true}

	log.Debugf("GLM Review called: type=%s, author_role=%d, api_key_configured=%v",
		content.ObjectType, content.Author.Role, len(r.Config.APIKey) > 0)

	if len(r.Config.APIKey) == 0 {
		log.Warnf("GLM API Key not configured, skipping review")
		return result
	}

	if content.Author.Role > 1 {
		log.Debugf("Author is admin/moderator (role=%d), skipping review", content.Author.Role)
		return result
	}

	shouldReview := false
	switch content.ObjectType {
	case "question":
		shouldReview = r.Config.ReviewQuestion
	case "answer":
		shouldReview = r.Config.ReviewAnswer
	case "comment":
		shouldReview = r.Config.ReviewComment
	}

	log.Debugf("Should review %s: %v (config: question=%v, answer=%v, comment=%v)",
		content.ObjectType, shouldReview,
		r.Config.ReviewQuestion, r.Config.ReviewAnswer, r.Config.ReviewComment)

	if !shouldReview {
		return result
	}

	// Combine title and content
	textToReview := content.Title + "\n" + content.Content

	// Truncate if content is too long (GLM API has token limits)
	maxLength := r.Config.MaxContentLength
	if maxLength <= 0 {
		maxLength = 8000 // Fallback to default
	}
	if len(textToReview) > maxLength {
		log.Warnf("Content too long (%d chars), truncating to %d chars for review", len(textToReview), maxLength)
		textToReview = textToReview[:maxLength]
	}

	approved, err := r.checkContent(textToReview)
	if err != nil {
		log.Errorf("GLM content review failed: %v", err)
		return handleReviewError(content, plugin.ReviewStatusNeedReview)
	}

	if approved {
		return result
	}

	if r.Config.SpamFiltering == "delete" {
		return handleReviewError(content, plugin.ReviewStatusDeleteDirectly)
	}

	return handleReviewError(content, plugin.ReviewStatusNeedReview)
}

func (r *Reviewer) checkContent(text string) (bool, error) {
	// Generate cache key from content hash
	hash := sha256.Sum256([]byte(text))
	cacheKey := hex.EncodeToString(hash[:])

	// Get cache TTL from config
	cacheTTL := time.Duration(r.Config.CacheTTL) * time.Minute
	if cacheTTL <= 0 {
		cacheTTL = 60 * time.Minute // Fallback to default
	}

	// Check cache first
	if approved, found := globalCache.get(cacheKey, cacheTTL); found {
		log.Debugf("Cache hit for content review")
		globalMetrics.recordCacheHit()
		globalMetrics.recordReview(approved)
		return approved, nil
	}

	// Update rate limiter config if needed
	rateLimit := r.Config.RateLimitRPS
	if rateLimit <= 0 {
		rateLimit = 10 // Fallback to default
	}
	globalRateLimiter.updateConfig(rateLimit)

	// Check rate limiter
	if !globalRateLimiter.allow() {
		log.Warnf("Rate limit exceeded, rejecting content for review")
		globalMetrics.recordRateLimitExceeded()
		return false, fmt.Errorf("rate limit exceeded")
	}

	maxRetries := r.Config.MaxRetries
	if maxRetries < 0 {
		maxRetries = 2 // Fallback to default
	}
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
			log.Debugf("Retrying GLM API call, attempt %d/%d", attempt+1, maxRetries+1)
		}

		approved, err := r.callGLMAPI(text)
		if err == nil {
			// Cache the result with configurable max size
			maxSize := r.Config.CacheMaxSize
			if maxSize <= 0 {
				maxSize = 1000 // Fallback to default
			}
			globalCache.set(cacheKey, approved, maxSize)
			globalMetrics.recordReview(approved)
			log.Infof("Content review completed: approved=%v, content_length=%d", approved, len(text))
			return approved, nil
		}

		// Check if it's a content filter error (code 1301)
		// If so, don't retry - immediately mark for review
		if _, isContentFilterError := err.(*ContentFilterError); isContentFilterError {
			log.Warnf("GLM content filter triggered, marking content for review: %v", err)
			globalMetrics.recordReview(false)
			return false, nil // Return false (needs review) without error
		}

		lastErr = err
		log.Warnf("GLM API call failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
	}

	globalMetrics.recordAPIError()
	return false, fmt.Errorf("GLM API failed after %d attempts: %w", maxRetries+1, lastErr)
}

func (r *Reviewer) callGLMAPI(text string) (bool, error) {
	timeout := time.Duration(r.Config.APITimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second // Fallback to default
	}
	client := &http.Client{Timeout: timeout}

	reqBody := map[string]interface{}{
		"model": "glm-4",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": fmt.Sprintf("请审核以下内容是否包含违规信息（如色情、暴力、政治敏感、违法等）。如果内容安全，只回复'safe'；如果内容违规，只回复'unsafe'。内容：\n%s", text),
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("POST", "https://open.bigmodel.cn/api/paas/v4/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.Config.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		// Try to parse the error response to check for content filter
		var errorResp GLMResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			// Check if it's a content filter error (code 1301)
			if errorResp.Error != nil && errorResp.Error.Code == "1301" {
				return false, &ContentFilterError{
					Message: errorResp.Error.Message,
					Code:    errorResp.Error.Code,
				}
			}
		}

		return false, fmt.Errorf("GLM API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var glmResp GLMResponse
	if err := json.Unmarshal(body, &glmResp); err != nil {
		return false, err
	}

	if glmResp.Error != nil {
		return false, fmt.Errorf("GLM API error: %s", glmResp.Error.Message)
	}

	if len(glmResp.Choices) > 0 {
		content := strings.TrimSpace(strings.ToLower(glmResp.Choices[0].Message.Content))
		// Check if content is safe
		if strings.Contains(content, "safe") || strings.Contains(content, "安全") {
			return true, nil
		}
		// Check if content is unsafe
		if strings.Contains(content, "unsafe") || strings.Contains(content, "违规") ||
			strings.Contains(content, "不安全") || strings.Contains(content, "不合规") {
			return false, nil
		}
		// If response is ambiguous, log it and treat as unsafe for safety
		log.Warnf("GLM API returned ambiguous response: %s", glmResp.Choices[0].Message.Content)
		return false, nil
	}

	return false, fmt.Errorf("no response from GLM API")
}

func (r *Reviewer) ConfigFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{
			Name:        "api_key",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigAPIKeyTitle),
			Description: plugin.MakeTranslator(i18n.ConfigAPIKeyDescription),
			Required:    true,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeText,
				Label:     plugin.MakeTranslator(i18n.ConfigAPIKeyLabel),
			},
			Value: r.Config.APIKey,
		},
		{
			Name:        "api_timeout",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigAPITimeoutTitle),
			Description: plugin.MakeTranslator(i18n.ConfigAPITimeoutDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigAPITimeoutLabel),
			},
			Value: r.Config.APITimeout,
		},
		{
			Name:        "max_content_length",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigMaxContentLengthTitle),
			Description: plugin.MakeTranslator(i18n.ConfigMaxContentLengthDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigMaxContentLengthLabel),
			},
			Value: r.Config.MaxContentLength,
		},
		{
			Name:        "cache_ttl",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigCacheTTLTitle),
			Description: plugin.MakeTranslator(i18n.ConfigCacheTTLDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigCacheTTLLabel),
			},
			Value: r.Config.CacheTTL,
		},
		{
			Name:        "cache_max_size",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigCacheMaxSizeTitle),
			Description: plugin.MakeTranslator(i18n.ConfigCacheMaxSizeDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigCacheMaxSizeLabel),
			},
			Value: r.Config.CacheMaxSize,
		},
		{
			Name:        "rate_limit_rps",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigRateLimitRPSTitle),
			Description: plugin.MakeTranslator(i18n.ConfigRateLimitRPSDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigRateLimitRPSLabel),
			},
			Value: r.Config.RateLimitRPS,
		},
		{
			Name:        "max_retries",
			Type:        plugin.ConfigTypeInput,
			Title:       plugin.MakeTranslator(i18n.ConfigMaxRetriesTitle),
			Description: plugin.MakeTranslator(i18n.ConfigMaxRetriesDescription),
			Required:    false,
			UIOptions: plugin.ConfigFieldUIOptions{
				InputType: plugin.InputTypeNumber,
				Label:     plugin.MakeTranslator(i18n.ConfigMaxRetriesLabel),
			},
			Value: r.Config.MaxRetries,
		},
		{
			Name:     "review_question",
			Type:     plugin.ConfigTypeSwitch,
			Title:    plugin.MakeTranslator(i18n.ConfigReviewQuestionTitle),
			Required: false,
			UIOptions: plugin.ConfigFieldUIOptions{
				Label: plugin.MakeTranslator(i18n.ConfigReviewQuestionLabel),
			},
			Value: r.Config.ReviewQuestion,
		},
		{
			Name:     "review_answer",
			Type:     plugin.ConfigTypeSwitch,
			Title:    plugin.MakeTranslator(i18n.ConfigReviewAnswerTitle),
			Required: false,
			UIOptions: plugin.ConfigFieldUIOptions{
				Label: plugin.MakeTranslator(i18n.ConfigReviewAnswerLabel),
			},
			Value: r.Config.ReviewAnswer,
		},
		{
			Name:     "review_comment",
			Type:     plugin.ConfigTypeSwitch,
			Title:    plugin.MakeTranslator(i18n.ConfigReviewCommentTitle),
			Required: false,
			UIOptions: plugin.ConfigFieldUIOptions{
				Label: plugin.MakeTranslator(i18n.ConfigReviewCommentLabel),
			},
			Value: r.Config.ReviewComment,
		},
		{
			Name:      "spam_filtering",
			Type:      plugin.ConfigTypeSelect,
			Title:     plugin.MakeTranslator(i18n.ConfigSpamFilteringTitle),
			Required:  false,
			UIOptions: plugin.ConfigFieldUIOptions{},
			Value:     r.Config.SpamFiltering,
			Options: []plugin.ConfigFieldOption{
				{
					Value: "review",
					Label: plugin.MakeTranslator(i18n.ConfigSpamFilteringReview),
				},
				{
					Value: "delete",
					Label: plugin.MakeTranslator(i18n.ConfigSpamFilteringDelete),
				},
			},
		},
	}
}

func (r *Reviewer) ConfigReceiver(config []byte) error {
	c := &ReviewerConfig{}
	_ = json.Unmarshal(config, c)
	r.Config = c
	return nil
}

// TestConnection tests the GLM API connection
func (r *Reviewer) TestConnection() error {
	if len(r.Config.APIKey) == 0 {
		return fmt.Errorf("API Key is not configured")
	}

	// Test with a simple safe text
	testText := "Hello, this is a test message."
	_, err := r.callGLMAPI(testText)
	if err != nil {
		return fmt.Errorf("API connection test failed: %w", err)
	}

	log.Infof("GLM API connection test successful")
	return nil
}

func handleReviewError(content *plugin.ReviewContent, reviewStatus plugin.ReviewStatus) *plugin.ReviewResult {
	return &plugin.ReviewResult{
		Approved:     false,
		ReviewStatus: reviewStatus,
		Reason:       plugin.TranslateWithData(myI18n.Language(content.Language), i18n.CommentNeedReview, nil),
	}
}
