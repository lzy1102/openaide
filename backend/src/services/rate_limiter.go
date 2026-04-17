package services

import (
	"math"
	"sync"
	"time"
)

// RateLimiter 限流器接口
type RateLimiter interface {
	// Allow 检查是否允许请求
	Allow(key string) bool
	// Remaining 获取剩余配额
	Remaining(key string) int
	// ResetAt 获取重置时间
	ResetAt(key string) time.Time
	// Increment 增加计数
	Increment(key string) int
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// RequestsPerWindow 时间窗口内允许的最大请求数
	RequestsPerWindow int
	// WindowSize 时间窗口大小
	WindowSize time.Duration
	// KeyPrefix 键前缀
	KeyPrefix string
	// CleanupInterval 清理间隔
	CleanupInterval time.Duration
}

// SlidingWindowLimiter 滑动窗口限流器
type SlidingWindowLimiter struct {
	config   RateLimitConfig
	windows  map[string]*slidingWindow
	mu       sync.RWMutex
	stopChan chan struct{}
}

type slidingWindow struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// NewSlidingWindowLimiter 创建滑动窗口限流器
func NewSlidingWindowLimiter(config RateLimitConfig) *SlidingWindowLimiter {
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Minute
	}

	limiter := &SlidingWindowLimiter{
		config:   config,
		windows:  make(map[string]*slidingWindow),
		stopChan: make(chan struct{}),
	}

	// 启动清理协程
	go limiter.cleanup()

	return limiter
}

// Allow 检查是否允许请求
func (l *SlidingWindowLimiter) Allow(key string) bool {
	return l.Remaining(key) > 0
}

// Remaining 获取剩余配额
func (l *SlidingWindowLimiter) Remaining(key string) int {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.config.WindowSize)

	// 移除过期的请求记录
	validIdx := 0
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			window.timestamps[validIdx] = ts
			validIdx++
		}
	}
	window.timestamps = window.timestamps[:validIdx]

	return l.config.RequestsPerWindow - len(window.timestamps)
}

// ResetAt 获取重置时间
func (l *SlidingWindowLimiter) ResetAt(key string) time.Time {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	if len(window.timestamps) == 0 {
		return time.Now()
	}

	// 最早的请求时间 + 窗口大小 = 重置时间
	oldest := window.timestamps[0]
	for _, ts := range window.timestamps {
		if ts.Before(oldest) {
			oldest = ts
		}
	}

	return oldest.Add(l.config.WindowSize)
}

// Increment 增加计数
func (l *SlidingWindowLimiter) Increment(key string) int {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.config.WindowSize)

	// 移除过期的请求记录
	validIdx := 0
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			window.timestamps[validIdx] = ts
			validIdx++
		}
	}
	window.timestamps = window.timestamps[:validIdx]

	// 检查是否超过限制
	if len(window.timestamps) >= l.config.RequestsPerWindow {
		return len(window.timestamps)
	}

	// 添加新请求
	window.timestamps = append(window.timestamps, now)
	return len(window.timestamps)
}

func (l *SlidingWindowLimiter) getWindow(key string) *slidingWindow {
	fullKey := l.config.KeyPrefix + key

	l.mu.RLock()
	window, exists := l.windows[fullKey]
	l.mu.RUnlock()

	if exists {
		return window
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 双重检查
	if window, exists = l.windows[fullKey]; exists {
		return window
	}

	window = &slidingWindow{
		timestamps: make([]time.Time, 0, l.config.RequestsPerWindow),
	}
	l.windows[fullKey] = window
	return window
}

// cleanup 定期清理过期窗口
func (l *SlidingWindowLimiter) cleanup() {
	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.doCleanup()
		case <-l.stopChan:
			return
		}
	}
}

func (l *SlidingWindowLimiter) doCleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-l.config.WindowSize)

	for key, window := range l.windows {
		window.mu.Lock()
		// 移除过期记录
		validIdx := 0
		for _, ts := range window.timestamps {
			if ts.After(cutoff) {
				window.timestamps[validIdx] = ts
				validIdx++
			}
		}
		window.timestamps = window.timestamps[:validIdx]

		// 如果窗口为空，删除整个窗口
		if len(window.timestamps) == 0 {
			delete(l.windows, key)
		}
		window.mu.Unlock()
	}
}

// Stop 停止限流器
func (l *SlidingWindowLimiter) Stop() {
	close(l.stopChan)
}

// TokenBucketLimiter 令牌桶限流器
type TokenBucketLimiter struct {
	config    RateLimitConfig
	buckets   map[string]*tokenBucket
	mu        sync.RWMutex
	stopChan  chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTokenBucketLimiter 创建令牌桶限流器
func NewTokenBucketLimiter(config RateLimitConfig) *TokenBucketLimiter {
	if config.CleanupInterval == 0 {
		config.CleanupInterval = time.Minute
	}

	limiter := &TokenBucketLimiter{
		config:   config,
		buckets:  make(map[string]*tokenBucket),
		stopChan: make(chan struct{}),
	}

	// 启动清理协程
	go limiter.cleanup()

	return limiter
}

// Allow 检查是否允许请求
func (l *TokenBucketLimiter) Allow(key string) bool {
	return l.Remaining(key) > 0
}

// Remaining 获取剩余配额
func (l *TokenBucketLimiter) Remaining(key string) int {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	l.refill(bucket)
	return int(bucket.tokens)
}

// ResetAt 获取重置时间
func (l *TokenBucketLimiter) ResetAt(key string) time.Time {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	l.refill(bucket)
	if bucket.tokens >= float64(l.config.RequestsPerWindow) {
		return time.Now()
	}

	// 计算填满需要的时间
	tokensNeeded := float64(l.config.RequestsPerWindow) - bucket.tokens
	rate := float64(l.config.RequestsPerWindow) / l.config.WindowSize.Seconds()
	timeNeeded := time.Duration(tokensNeeded/rate) * time.Second

	return time.Now().Add(timeNeeded)
}

// Increment 增加计数（消费令牌）
func (l *TokenBucketLimiter) Increment(key string) int {
	bucket := l.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	l.refill(bucket)

	if bucket.tokens >= 1 {
		bucket.tokens--
	}

	return int(bucket.tokens)
}

func (l *TokenBucketLimiter) getBucket(key string) *tokenBucket {
	fullKey := l.config.KeyPrefix + key

	l.mu.RLock()
	bucket, exists := l.buckets[fullKey]
	l.mu.RUnlock()

	if exists {
		return bucket
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if bucket, exists = l.buckets[fullKey]; exists {
		return bucket
	}

	bucket = &tokenBucket{
		tokens:     float64(l.config.RequestsPerWindow),
		lastUpdate: time.Now(),
	}
	l.buckets[fullKey] = bucket
	return bucket
}

func (l *TokenBucketLimiter) refill(bucket *tokenBucket) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdate)

	// 计算新增令牌
	rate := float64(l.config.RequestsPerWindow) / l.config.WindowSize.Seconds()
	newTokens := elapsed.Seconds() * rate

	bucket.tokens = math.Min(float64(l.config.RequestsPerWindow), bucket.tokens+newTokens)
	bucket.lastUpdate = now
}

func (l *TokenBucketLimiter) cleanup() {
	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.doCleanup()
		case <-l.stopChan:
			return
		}
	}
}

func (l *TokenBucketLimiter) doCleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清理长时间未使用的桶
	cutoff := time.Now().Add(-l.config.WindowSize * 2)

	for key, bucket := range l.buckets {
		bucket.mu.Lock()
		if bucket.lastUpdate.Before(cutoff) {
			delete(l.buckets, key)
		}
		bucket.mu.Unlock()
	}
}

// Stop 停止限流器
func (l *TokenBucketLimiter) Stop() {
	close(l.stopChan)
}

// RateLimitService 限流服务
type RateLimitService struct {
	ipLimiter     RateLimiter
	userLimiter   RateLimiter
	apiKeyLimiter RateLimiter
	customLimiters map[string]RateLimiter
	mu            sync.RWMutex
}

// RateLimitServiceConfig 限流服务配置
type RateLimitServiceConfig struct {
	// IP 限流配置
	IPRequestsPerMinute int
	// 用户限流配置
	UserRequestsPerMinute int
	// API Key 限流配置
	APIKeyRequestsPerMinute int
	// 自定义限流规则
	CustomRules map[string]RateLimitConfig
}

// NewRateLimitService 创建限流服务
func NewRateLimitService(config RateLimitServiceConfig) *RateLimitService {
	if config.IPRequestsPerMinute == 0 {
		config.IPRequestsPerMinute = 100
	}
	if config.UserRequestsPerMinute == 0 {
		config.UserRequestsPerMinute = 200
	}
	if config.APIKeyRequestsPerMinute == 0 {
		config.APIKeyRequestsPerMinute = 500
	}

	service := &RateLimitService{
		ipLimiter: NewSlidingWindowLimiter(RateLimitConfig{
			RequestsPerWindow: config.IPRequestsPerMinute,
			WindowSize:        time.Minute,
			KeyPrefix:         "ip:",
		}),
		userLimiter: NewSlidingWindowLimiter(RateLimitConfig{
			RequestsPerWindow: config.UserRequestsPerMinute,
			WindowSize:        time.Minute,
			KeyPrefix:         "user:",
		}),
		apiKeyLimiter: NewSlidingWindowLimiter(RateLimitConfig{
			RequestsPerWindow: config.APIKeyRequestsPerMinute,
			WindowSize:        time.Minute,
			KeyPrefix:         "apikey:",
		}),
		customLimiters: make(map[string]RateLimiter),
	}

	// 添加自定义规则
	for name, ruleConfig := range config.CustomRules {
		service.customLimiters[name] = NewSlidingWindowLimiter(ruleConfig)
	}

	return service
}

// CheckIP 检查 IP 限流
func (s *RateLimitService) CheckIP(ip string) (bool, int, time.Time) {
	remaining := s.ipLimiter.Remaining(ip)
	resetAt := s.ipLimiter.ResetAt(ip)
	allowed := remaining > 0

	if allowed {
		s.ipLimiter.Increment(ip)
		remaining--
	}

	return allowed, remaining, resetAt
}

// CheckUser 检查用户限流
func (s *RateLimitService) CheckUser(userID string) (bool, int, time.Time) {
	remaining := s.userLimiter.Remaining(userID)
	resetAt := s.userLimiter.ResetAt(userID)
	allowed := remaining > 0

	if allowed {
		s.userLimiter.Increment(userID)
		remaining--
	}

	return allowed, remaining, resetAt
}

// CheckAPIKey 检查 API Key 限流
func (s *RateLimitService) CheckAPIKey(apiKeyID string) (bool, int, time.Time) {
	remaining := s.apiKeyLimiter.Remaining(apiKeyID)
	resetAt := s.apiKeyLimiter.ResetAt(apiKeyID)
	allowed := remaining > 0

	if allowed {
		s.apiKeyLimiter.Increment(apiKeyID)
		remaining--
	}

	return allowed, remaining, resetAt
}

// CheckCustom 检查自定义限流规则
func (s *RateLimitService) CheckCustom(ruleName, key string) (bool, int, time.Time) {
	s.mu.RLock()
	limiter, exists := s.customLimiters[ruleName]
	s.mu.RUnlock()

	if !exists {
		return true, 0, time.Now()
	}

	remaining := limiter.Remaining(key)
	resetAt := limiter.ResetAt(key)
	allowed := remaining > 0

	if allowed {
		limiter.Increment(key)
		remaining--
	}

	return allowed, remaining, resetAt
}

// AddCustomRule 添加自定义限流规则
func (s *RateLimitService) AddCustomRule(name string, config RateLimitConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.customLimiters[name] = NewSlidingWindowLimiter(config)
}

// RemoveCustomRule 移除自定义限流规则
func (s *RateLimitService) RemoveCustomRule(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.customLimiters, name)
}

// Stop 停止所有限流器
func (s *RateLimitService) Stop() {
	if limiter, ok := s.ipLimiter.(*SlidingWindowLimiter); ok {
		limiter.Stop()
	}
	if limiter, ok := s.userLimiter.(*SlidingWindowLimiter); ok {
		limiter.Stop()
	}
	if limiter, ok := s.apiKeyLimiter.(*SlidingWindowLimiter); ok {
		limiter.Stop()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, limiter := range s.customLimiters {
		if l, ok := limiter.(*SlidingWindowLimiter); ok {
			l.Stop()
		}
	}
}

// RateLimitResult 限流结果
type RateLimitResult struct {
	Allowed    bool      `json:"allowed"`
	Remaining  int       `json:"remaining"`
	Limit      int       `json:"limit"`
	ResetAt    time.Time `json:"reset_at"`
	RetryAfter int       `json:"retry_after,omitempty"` // 秒
}

// SimpleRateLimitConfig 简化限流配置
type SimpleRateLimitConfig struct {
	RequestsPerWindow int
	WindowSize        time.Duration
	KeyPrefix         string
}

// SlidingWindowLimiterSimple 简化的滑动窗口限流器
type SlidingWindowLimiterSimple struct {
	config  SimpleRateLimitConfig
	windows map[string]*simpleWindow
	mu      sync.RWMutex
}

type simpleWindow struct {
	timestamps []time.Time
	mu         sync.Mutex
}

// NewSimpleSlidingWindowLimiter 创建简化的滑动窗口限流器
func NewSimpleSlidingWindowLimiter(config SimpleRateLimitConfig) *SlidingWindowLimiterSimple {
	return &SlidingWindowLimiterSimple{
		config:  config,
		windows: make(map[string]*simpleWindow),
	}
}

// Remaining 获取剩余配额
func (l *SlidingWindowLimiterSimple) Remaining(key string) int {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.config.WindowSize)

	// 移除过期的请求记录
	validIdx := 0
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			window.timestamps[validIdx] = ts
			validIdx++
		}
	}
	window.timestamps = window.timestamps[:validIdx]

	return l.config.RequestsPerWindow - len(window.timestamps)
}

// ResetAt 获取重置时间
func (l *SlidingWindowLimiterSimple) ResetAt(key string) time.Time {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	if len(window.timestamps) == 0 {
		return time.Now().Add(l.config.WindowSize)
	}

	// 最早的请求时间 + 窗口大小 = 重置时间
	oldest := window.timestamps[0]
	for _, ts := range window.timestamps {
		if ts.Before(oldest) {
			oldest = ts
		}
	}

	return oldest.Add(l.config.WindowSize)
}

// Increment 增加计数
func (l *SlidingWindowLimiterSimple) Increment(key string) {
	window := l.getWindow(key)
	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.config.WindowSize)

	// 移除过期的请求记录
	validIdx := 0
	for _, ts := range window.timestamps {
		if ts.After(cutoff) {
			window.timestamps[validIdx] = ts
			validIdx++
		}
	}
	window.timestamps = window.timestamps[:validIdx]

	// 添加新请求
	window.timestamps = append(window.timestamps, now)
}

func (l *SlidingWindowLimiterSimple) getWindow(key string) *simpleWindow {
	fullKey := l.config.KeyPrefix + key

	l.mu.RLock()
	window, exists := l.windows[fullKey]
	l.mu.RUnlock()

	if exists {
		return window
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if window, exists = l.windows[fullKey]; exists {
		return window
	}

	window = &simpleWindow{
		timestamps: make([]time.Time, 0, l.config.RequestsPerWindow),
	}
	l.windows[fullKey] = window
	return window
}
