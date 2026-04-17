package services

import (
	"encoding/json"
	"fmt"
	"time"

	ledisconfig "github.com/ledisdb/ledisdb/config"
	"github.com/ledisdb/ledisdb/ledis"

	"openaide/backend/src/config"
)

// LedisCacheService 基于 LedisDB 的缓存服务（兼容 Redis 协议）
type LedisCacheService struct {
	ldb     *ledis.Ledis
	db      *ledis.DB
	dataDir string
}

// NewLedisCacheService 创建 LedisDB 缓存服务
func NewLedisCacheService(dataDir string) (*LedisCacheService, error) {
	if dataDir == "" {
		// 使用统一的数据目录 ~/.openaide/data/ledis
		dataDir = config.DefaultPaths.LedisDir
	}

	cfg := ledisconfig.NewConfigDefault()
	cfg.DataDir = dataDir
	cfg.Databases = 1

	// 使用 LevelDB 后端（性能较好）
	cfg.DBName = "leveldb"

	ldb, err := ledis.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open ledisdb: %w", err)
	}

	db, err := ldb.Select(0)
	if err != nil {
		return nil, fmt.Errorf("failed to select db: %w", err)
	}

	return &LedisCacheService{
		ldb:     ldb,
		db:      db,
		dataDir: dataDir,
	}, nil
}

// Close 关闭数据库
func (s *LedisCacheService) Close() error {
	if s.ldb != nil {
		s.ldb.Close()
	}
	return nil
}

// ==================== 基础 KV 操作 ====================

// Get 获取值
func (s *LedisCacheService) Get(key string) (interface{}, bool) {
	data, err := s.db.Get([]byte(key))
	if err != nil || data == nil {
		return nil, false
	}

	// 尝试反序列化
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		// 如果不是 JSON，返回原始字符串
		return string(data), true
	}
	return value, true
}

// GetString 获取字符串值
func (s *LedisCacheService) GetString(key string) (string, bool) {
	data, err := s.db.Get([]byte(key))
	if err != nil || data == nil {
		return "", false
	}
	return string(data), true
}

// GetBytes 获取字节值
func (s *LedisCacheService) GetBytes(key string) ([]byte, bool) {
	data, err := s.db.Get([]byte(key))
	if err != nil || data == nil {
		return nil, false
	}
	return data, true
}

// Set 设置值（无过期时间）
func (s *LedisCacheService) Set(key string, value interface{}) error {
	data, err := s.serialize(value)
	if err != nil {
		return err
	}
	return s.db.Set([]byte(key), data)
}

// SetWithExpiration 设置值带过期时间
func (s *LedisCacheService) SetWithExpiration(key string, value interface{}, expiration time.Duration) error {
	data, err := s.serialize(value)
	if err != nil {
		return err
	}

	// LedisDB 使用秒或毫秒
	seconds := int64(expiration.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	err = s.db.SetEX([]byte(key), seconds, data)
	return err
}

// SetNX 仅在不存在时设置（用于分布式锁）
func (s *LedisCacheService) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	data, err := s.serialize(value)
	if err != nil {
		return false, err
	}

	// 使用 SetNX 命令
	n, err := s.db.SetNX([]byte(key), data)
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}

	// 设置过期时间
	if expiration > 0 {
		seconds := int64(expiration.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		s.db.Expire([]byte(key), seconds)
	}

	return true, nil
}

// Delete 删除键
func (s *LedisCacheService) Delete(key string) error {
	_, err := s.db.Del([]byte(key))
	return err
}

// DeleteMultiple 删除多个键
func (s *LedisCacheService) DeleteMultiple(keys ...string) error {
	for _, key := range keys {
		_, err := s.db.Del([]byte(key))
		if err != nil {
			return err
		}
	}
	return nil
}

// Exists 检查键是否存在
func (s *LedisCacheService) Exists(key string) bool {
	_, err := s.db.Get([]byte(key))
	return err == nil
}

// TTL 获取剩余过期时间
func (s *LedisCacheService) TTL(key string) time.Duration {
	ttl, err := s.db.TTL([]byte(key))
	if err != nil {
		return -1
	}
	return time.Duration(ttl) * time.Second
}

// Expire 设置过期时间
func (s *LedisCacheService) Expire(key string, expiration time.Duration) error {
	seconds := int64(expiration.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	_, err := s.db.Expire([]byte(key), seconds)
	return err
}

// ==================== 高级操作 ====================

// Incr 自增
func (s *LedisCacheService) Incr(key string) (int64, error) {
	return s.db.Incr([]byte(key))
}

// IncrBy 按值自增
func (s *LedisCacheService) IncrBy(key string, delta int64) (int64, error) {
	return s.db.IncrBy([]byte(key), delta)
}

// Decr 自减
func (s *LedisCacheService) Decr(key string) (int64, error) {
	return s.db.Decr([]byte(key))
}

// ==================== Hash 操作（用于存储对象）====================

// HSet 设置 Hash 字段
func (s *LedisCacheService) HSet(key, field string, value interface{}) error {
	data, err := s.serialize(value)
	if err != nil {
		return err
	}
	_, err = s.db.HSet([]byte(key), []byte(field), data)
	return err
}

// HGet 获取 Hash 字段
func (s *LedisCacheService) HGet(key, field string) (interface{}, bool) {
	data, err := s.db.HGet([]byte(key), []byte(field))
	if err != nil || data == nil {
		return nil, false
	}

	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data), true
	}
	return value, true
}

// HGetAll 获取所有 Hash 字段
func (s *LedisCacheService) HGetAll(key string) (map[string]interface{}, error) {
	pairs, err := s.db.HGetAll([]byte(key))
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, pair := range pairs {
		field := string(pair.Field)
		var value interface{}
		if err := json.Unmarshal(pair.Value, &value); err != nil {
			result[field] = string(pair.Value)
		} else {
			result[field] = value
		}
	}
	return result, nil
}

// HDel 删除 Hash 字段
func (s *LedisCacheService) HDel(key string, fields ...string) error {
	for _, field := range fields {
		_, err := s.db.HDel([]byte(key), []byte(field))
		if err != nil {
			return err
		}
	}
	return nil
}

// ==================== List 操作 ====================

// LPush 从左侧推入列表
func (s *LedisCacheService) LPush(key string, values ...interface{}) (int64, error) {
	var count int64
	for _, value := range values {
		data, err := s.serialize(value)
		if err != nil {
			return count, err
		}
		n, err := s.db.LPush([]byte(key), data)
		if err != nil {
			return count, err
		}
		count = n
	}
	return count, nil
}

// RPush 从右侧推入列表
func (s *LedisCacheService) RPush(key string, values ...interface{}) (int64, error) {
	var count int64
	for _, value := range values {
		data, err := s.serialize(value)
		if err != nil {
			return count, err
		}
		n, err := s.db.RPush([]byte(key), data)
		if err != nil {
			return count, err
		}
		count = n
	}
	return count, nil
}

// LPop 从左侧弹出
func (s *LedisCacheService) LPop(key string) (interface{}, bool) {
	data, err := s.db.LPop([]byte(key))
	if err != nil || data == nil {
		return nil, false
	}

	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data), true
	}
	return value, true
}

// RPop 从右侧弹出
func (s *LedisCacheService) RPop(key string) (interface{}, bool) {
	data, err := s.db.RPop([]byte(key))
	if err != nil || data == nil {
		return nil, false
	}

	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data), true
	}
	return value, true
}

// LRange 获取列表范围
func (s *LedisCacheService) LRange(key string, start, stop int) ([]interface{}, error) {
	datas, err := s.db.LRange([]byte(key), int32(start), int32(stop))
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(datas))
	for i, data := range datas {
		var value interface{}
		if err := json.Unmarshal(data, &value); err != nil {
			result[i] = string(data)
		} else {
			result[i] = value
		}
	}
	return result, nil
}

// LLen 获取列表长度
func (s *LedisCacheService) LLen(key string) (int64, error) {
	return s.db.LLen([]byte(key))
}

// ==================== Set 操作 ====================

// SAdd 添加集合成员
func (s *LedisCacheService) SAdd(key string, members ...interface{}) (int64, error) {
	var count int64
	for _, member := range members {
		data, err := s.serialize(member)
		if err != nil {
			return count, err
		}
		n, err := s.db.SAdd([]byte(key), data)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

// SMembers 获取所有集合成员
func (s *LedisCacheService) SMembers(key string) ([]interface{}, error) {
	datas, err := s.db.SMembers([]byte(key))
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(datas))
	for i, data := range datas {
		var value interface{}
		if err := json.Unmarshal(data, &value); err != nil {
			result[i] = string(data)
		} else {
			result[i] = value
		}
	}
	return result, nil
}

// SRem 移除集合成员
func (s *LedisCacheService) SRem(key string, members ...interface{}) (int64, error) {
	var count int64
	for _, member := range members {
		data, err := s.serialize(member)
		if err != nil {
			return count, err
		}
		n, err := s.db.SRem([]byte(key), data)
		if err != nil {
			return count, err
		}
		count += n
	}
	return count, nil
}

// SIsMember 检查是否是集合成员
func (s *LedisCacheService) SIsMember(key string, member interface{}) (bool, error) {
	data, err := s.serialize(member)
	if err != nil {
		return false, err
	}
	n, err := s.db.SIsMember([]byte(key), data)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ==================== 序列化辅助方法 ====================

// serialize 序列化值
func (s *LedisCacheService) serialize(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return json.Marshal(value)
	}
}

// ==================== 批量操作 ====================

// MGet 批量获取
func (s *LedisCacheService) MGet(keys ...string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for _, key := range keys {
		if value, found := s.Get(key); found {
			result[key] = value
		}
	}
	return result, nil
}

// MSet 批量设置
func (s *LedisCacheService) MSet(kv map[string]interface{}) error {
	for key, value := range kv {
		if err := s.Set(key, value); err != nil {
			return err
		}
	}
	return nil
}

// Keys 获取匹配的键（谨慎使用，性能较低）
func (s *LedisCacheService) Keys(pattern string) ([]string, error) {
	// LedisDB 不直接支持 Keys 命令，这里返回空
	// 如果需要，可以使用 Scan 遍历
	return []string{}, nil
}

// FlushDB 清空当前数据库
func (s *LedisCacheService) FlushDB() error {
	// LedisDB 没有直接的 FlushDB，使用 FlushAll
	return s.ldb.FlushAll()
}

// Info 获取数据库信息
func (s *LedisCacheService) Info() (map[string]interface{}, error) {
	info := map[string]interface{}{
		"data_dir": s.dataDir,
		"type":     "ledisdb",
		"backend":  "leveldb",
	}
	return info, nil
}
