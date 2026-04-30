package services

import (
	"time"

	"openaide/backend/src/services/storage"
)

// CacheService 缓存服务
type CacheService struct {
	provider storage.CacheProvider
}

// NewCacheService 创建缓存服务实例（默认内存缓存）
func NewCacheService() *CacheService {
	return &CacheService{
		provider: storage.NewMemoryCache(5*time.Minute, 10*time.Minute),
	}
}

// NewCacheServiceWithProvider 使用指定提供者创建缓存服务
func NewCacheServiceWithProvider(provider storage.CacheProvider) *CacheService {
	return &CacheService{provider: provider}
}

// Set 设置缓存
func (s *CacheService) Set(key string, value interface{}, expiration time.Duration) {
	s.provider.Set(key, value, expiration)
}

// Get 获取缓存
func (s *CacheService) Get(key string) (interface{}, bool) {
	return s.provider.Get(key)
}

// Delete 删除缓存
func (s *CacheService) Delete(key string) {
	s.provider.Delete(key)
}

// Flush 清空缓存
func (s *CacheService) Flush() {
	s.provider.Flush()
}

// ItemCount 获取缓存项数量
func (s *CacheService) ItemCount() int {
	return s.provider.ItemCount()
}
