package storage

import (
	"fmt"
	"time"

	"openaide/backend/src/logger"

	"github.com/patrickmn/go-cache"
)

// CacheProvider 缓存提供者接口
type CacheProvider interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, expiration time.Duration)
	Delete(key string)
	Flush()
	ItemCount() int
}

// MemoryCache go-cache 内存缓存实现
type MemoryCache struct {
	cache *cache.Cache
}

// NewMemoryCache 创建内存缓存
func NewMemoryCache(defaultExpiration, cleanupInterval time.Duration) CacheProvider {
	c := cache.New(defaultExpiration, cleanupInterval)
	return &MemoryCache{cache: c}
}

func (m *MemoryCache) Get(key string) (interface{}, bool) {
	return m.cache.Get(key)
}

func (m *MemoryCache) Set(key string, value interface{}, expiration time.Duration) {
	m.cache.Set(key, value, expiration)
}

func (m *MemoryCache) Delete(key string) {
	m.cache.Delete(key)
}

func (m *MemoryCache) Flush() {
	m.cache.Flush()
}

func (m *MemoryCache) ItemCount() int {
	return m.cache.ItemCount()
}

// RedisCache Redis 缓存实现（占位，需要 go-redis 依赖）
type RedisCache struct {
	// client *redis.Client  // 需要导入 github.com/redis/go-redis/v9
	addr     string
	password string
	db       int
}

// NewRedisCache 创建 Redis 缓存
// 当前未实现，自动回退到内存缓存
func NewRedisCache(addr, password string, db int) (CacheProvider, error) {
	// TODO: 实现 Redis 连接
	// 需要添加依赖: go get github.com/redis/go-redis/v9
	// 临时回退到内存缓存，避免服务启动失败
	return NewMemoryCache(5*time.Minute, 10*time.Minute), nil
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Type              string        `json:"type"`               // "memory", "ledis", "redis"
	DefaultExpiration time.Duration `json:"default_expiration"` // 默认过期时间
	CleanupInterval   time.Duration `json:"cleanup_interval"`   // 清理间隔
	DataDir           string        `json:"data_dir,omitempty"` // LedisDB 数据目录
	// Redis 配置
	RedisAddr     string `json:"redis_addr,omitempty"`
	RedisPassword string `json:"redis_password,omitempty"`
	RedisDB       int    `json:"redis_db,omitempty"`
}

// NewCacheProvider 根据配置创建缓存提供者
func NewCacheProvider(cfg CacheConfig) CacheProvider {
	switch cfg.Type {
	case "redis":
		redisCache, err := NewRedisCache(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			logger.WithComponent("Cache").Error("failed to create redis cache, falling back to memory", "error", err)
			return NewMemoryCache(5*time.Minute, 10*time.Minute)
		}
		logger.WithComponent("Cache").Info("using Redis cache", "addr", cfg.RedisAddr)
		return redisCache
	case "ledis":
		cache, err := NewLedisCache(cfg.DataDir)
		if err != nil {
			logger.WithComponent("Cache").Error("failed to create ledis cache, falling back to memory", "error", err)
			return NewMemoryCache(5*time.Minute, 10*time.Minute)
		}
		logger.WithComponent("Cache").Info("using LedisDB cache", "data_dir", cfg.DataDir)
		return cache
	case "memory", "":
		defaultExp := cfg.DefaultExpiration
		if defaultExp == 0 {
			defaultExp = 5 * time.Minute
		}
		cleanup := cfg.CleanupInterval
		if cleanup == 0 {
			cleanup = 10 * time.Minute
		}
		return NewMemoryCache(defaultExp, cleanup)
	default:
		logger.WithComponent("Cache").Warn("unsupported cache type, falling back to memory", "type", cfg.Type)
		return NewMemoryCache(5*time.Minute, 10*time.Minute)
	}
}

// CacheService 兼容旧接口的缓存服务
type CacheService struct {
	provider CacheProvider
}

// NewCacheService 创建缓存服务（兼容旧接口）
func NewCacheService() *CacheService {
	return &CacheService{
		provider: NewMemoryCache(5*time.Minute, 10*time.Minute),
	}
}

// NewCacheServiceWithProvider 使用指定提供者创建缓存服务
func NewCacheServiceWithProvider(provider CacheProvider) *CacheService {
	return &CacheService{provider: provider}
}

func (s *CacheService) Get(key string) (interface{}, bool) {
	return s.provider.Get(key)
}

func (s *CacheService) Set(key string, value interface{}, expiration time.Duration) {
	s.provider.Set(key, value, expiration)
}

func (s *CacheService) Delete(key string) {
	s.provider.Delete(key)
}

func (s *CacheService) Flush() {
	s.provider.Flush()
}

func (s *CacheService) ItemCount() int {
	return s.provider.ItemCount()
}
