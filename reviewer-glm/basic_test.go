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
	"fmt"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(2, 100*time.Millisecond)

	// Should allow first 2 requests
	if !rl.allow() {
		t.Error("Expected first request to be allowed")
	}
	if !rl.allow() {
		t.Error("Expected second request to be allowed")
	}

	// Should block third request
	if rl.allow() {
		t.Error("Expected third request to be blocked")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should allow after refill
	if !rl.allow() {
		t.Error("Expected request to be allowed after refill")
	}
}

func TestCache(t *testing.T) {
	cache := &reviewCache{
		entries: make(map[string]cacheEntry),
	}

	// Test cache miss
	if _, found := cache.get("key1", 1*time.Hour); found {
		t.Error("Expected cache miss")
	}

	// Test cache set and hit
	cache.set("key1", true, 1000)
	if approved, found := cache.get("key1", 1*time.Hour); !found || !approved {
		t.Error("Expected cache hit with approved=true")
	}

	// Test cache expiration
	cache.entries["key2"] = cacheEntry{
		approved:  false,
		timestamp: time.Now().Add(-2 * time.Hour),
	}
	if _, found := cache.get("key2", 1*time.Hour); found {
		t.Error("Expected cache miss for expired entry")
	}
}

func TestMetrics(t *testing.T) {
	metrics := &reviewMetrics{}

	// Test initial state
	stats := metrics.getStats()
	if stats["total_reviews"] != 0 {
		t.Error("Expected initial total_reviews to be 0")
	}

	// Test recording reviews
	metrics.recordReview(true)
	metrics.recordReview(false)
	metrics.recordReview(true)

	stats = metrics.getStats()
	if stats["total_reviews"] != 3 {
		t.Errorf("Expected total_reviews=3, got %d", stats["total_reviews"])
	}
	if stats["approved_reviews"] != 2 {
		t.Errorf("Expected approved_reviews=2, got %d", stats["approved_reviews"])
	}
	if stats["rejected_reviews"] != 1 {
		t.Errorf("Expected rejected_reviews=1, got %d", stats["rejected_reviews"])
	}

	// Test cache hits
	metrics.recordCacheHit()
	stats = metrics.getStats()
	if stats["cache_hits"] != 1 {
		t.Errorf("Expected cache_hits=1, got %d", stats["cache_hits"])
	}

	// Test API errors
	metrics.recordAPIError()
	stats = metrics.getStats()
	if stats["api_errors"] != 1 {
		t.Errorf("Expected api_errors=1, got %d", stats["api_errors"])
	}

	// Test rate limit exceeded
	metrics.recordRateLimitExceeded()
	stats = metrics.getStats()
	if stats["rate_limit_exceeded"] != 1 {
		t.Errorf("Expected rate_limit_exceeded=1, got %d", stats["rate_limit_exceeded"])
	}
}

func TestConfigReceiver(t *testing.T) {
	reviewer := &Reviewer{
		Config: &ReviewerConfig{},
	}

	configJSON := []byte(`{
		"api_key": "test-key",
		"review_question": true,
		"review_answer": false,
		"review_comment": true,
		"spam_filtering": "delete",
		"api_timeout": 60
	}`)

	err := reviewer.ConfigReceiver(configJSON)
	if err != nil {
		t.Errorf("ConfigReceiver failed: %v", err)
	}

	if reviewer.Config.APIKey != "test-key" {
		t.Errorf("Expected APIKey=test-key, got %s", reviewer.Config.APIKey)
	}
	if !reviewer.Config.ReviewQuestion {
		t.Error("Expected ReviewQuestion=true")
	}
	if reviewer.Config.ReviewAnswer {
		t.Error("Expected ReviewAnswer=false")
	}
	if !reviewer.Config.ReviewComment {
		t.Error("Expected ReviewComment=true")
	}
	if reviewer.Config.SpamFiltering != "delete" {
		t.Errorf("Expected SpamFiltering=delete, got %s", reviewer.Config.SpamFiltering)
	}
	if reviewer.Config.APITimeout != 60 {
		t.Errorf("Expected APITimeout=60, got %d", reviewer.Config.APITimeout)
	}
}

func TestMinFunction(t *testing.T) {
	if min(5, 10) != 5 {
		t.Error("min(5, 10) should return 5")
	}
	if min(10, 5) != 5 {
		t.Error("min(10, 5) should return 5")
	}
	if min(7, 7) != 7 {
		t.Error("min(7, 7) should return 7")
	}
}

func TestConfigReceiverWithAllOptions(t *testing.T) {
	reviewer := &Reviewer{
		Config: &ReviewerConfig{},
	}

	configJSON := []byte(`{
		"api_key": "test-key-123",
		"review_question": true,
		"review_answer": false,
		"review_comment": true,
		"spam_filtering": "delete",
		"api_timeout": 45,
		"max_content_length": 10000,
		"cache_ttl": 120,
		"cache_max_size": 2000,
		"rate_limit_rps": 20
	}`)

	err := reviewer.ConfigReceiver(configJSON)
	if err != nil {
		t.Errorf("ConfigReceiver failed: %v", err)
	}

	if reviewer.Config.APIKey != "test-key-123" {
		t.Errorf("Expected APIKey=test-key-123, got %s", reviewer.Config.APIKey)
	}
	if reviewer.Config.APITimeout != 45 {
		t.Errorf("Expected APITimeout=45, got %d", reviewer.Config.APITimeout)
	}
	if reviewer.Config.MaxContentLength != 10000 {
		t.Errorf("Expected MaxContentLength=10000, got %d", reviewer.Config.MaxContentLength)
	}
	if reviewer.Config.CacheTTL != 120 {
		t.Errorf("Expected CacheTTL=120, got %d", reviewer.Config.CacheTTL)
	}
	if reviewer.Config.CacheMaxSize != 2000 {
		t.Errorf("Expected CacheMaxSize=2000, got %d", reviewer.Config.CacheMaxSize)
	}
	if reviewer.Config.RateLimitRPS != 20 {
		t.Errorf("Expected RateLimitRPS=20, got %d", reviewer.Config.RateLimitRPS)
	}
}

func TestCacheWithCustomTTL(t *testing.T) {
	cache := &reviewCache{
		entries: make(map[string]cacheEntry),
	}

	// Set an entry
	cache.set("key1", true, 1000)

	// Should find it with long TTL
	if _, found := cache.get("key1", 1*time.Hour); !found {
		t.Error("Expected cache hit with long TTL")
	}

	// Should not find it with very short TTL
	cache.entries["key1"] = cacheEntry{
		approved:  true,
		timestamp: time.Now().Add(-2 * time.Second),
	}
	if _, found := cache.get("key1", 1*time.Millisecond); found {
		t.Error("Expected cache miss with expired TTL")
	}
}

func TestCacheMaxSizeEnforcement(t *testing.T) {
	cache := &reviewCache{
		entries: make(map[string]cacheEntry),
	}

	// Add entries up to max size
	maxSize := 5
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		cache.set(key, true, maxSize)
	}

	// Cache should not exceed max size
	if len(cache.entries) > maxSize {
		t.Errorf("Cache size %d exceeds max size %d", len(cache.entries), maxSize)
	}
}

func TestRateLimiterUpdateConfig(t *testing.T) {
	rl := newRateLimiter(5, 100*time.Millisecond)

	// Use up initial tokens
	for i := 0; i < 5; i++ {
		if !rl.allow() {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// Should be blocked
	if rl.allow() {
		t.Error("Expected request to be blocked")
	}

	// Update config to allow more
	rl.updateConfig(10)

	// Should still be blocked (tokens not refilled yet)
	if rl.allow() {
		t.Error("Expected request to still be blocked")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should be allowed now
	if !rl.allow() {
		t.Error("Expected request to be allowed after refill")
	}
}
