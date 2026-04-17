package services

import (
	"time"

	"github.com/patrickmn/go-cache"
)

// CacheService 缓存服务
type CacheService struct {
	cache *cache.Cache
}

// NewCacheService 创建缓存服务实例
func NewCacheService() *CacheService {
	// 创建缓存，默认过期时间5分钟，每10分钟清理一次过期项
	c := cache.New(5*time.Minute, 10*time.Minute)
	return &CacheService{cache: c}
}

// Set 设置缓存
func (s *CacheService) Set(key string, value interface{}, expiration time.Duration) {
	s.cache.Set(key, value, expiration)
}

// Get 获取缓存
func (s *CacheService) Get(key string) (interface{}, bool) {
	return s.cache.Get(key)
}

// Delete 删除缓存
func (s *CacheService) Delete(key string) {
	s.cache.Delete(key)
}

// Flush 清空缓存
func (s *CacheService) Flush() {
	s.cache.Flush()
}

// ItemCount 获取缓存项数量
func (s *CacheService) ItemCount() int {
	return s.cache.ItemCount()
}
