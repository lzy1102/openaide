package storage

import (
	"encoding/json"
	"fmt"
	"time"

	ledisconfig "github.com/ledisdb/ledisdb/config"
	"github.com/ledisdb/ledisdb/ledis"

	"openaide/backend/src/config"
)

// LedisCache LedisDB 缓存实现（兼容 CacheProvider 接口）
// 优势：数据持久化、兼容 Redis 协议、未来可平滑迁移到 Redis
type LedisCache struct {
	ldb     *ledis.Ledis
	db      *ledis.DB
	dataDir string
}

// NewLedisCache 创建 LedisDB 缓存
func NewLedisCache(dataDir string) (CacheProvider, error) {
	if dataDir == "" {
		dataDir = config.DefaultPaths.LedisDir
	}

	cfg := ledisconfig.NewConfigDefault()
	cfg.DataDir = dataDir
	cfg.Databases = 1
	cfg.DBName = "leveldb"

	ldb, err := ledis.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open ledisdb: %w", err)
	}

	db, err := ldb.Select(0)
	if err != nil {
		ldb.Close()
		return nil, fmt.Errorf("failed to select db: %w", err)
	}

	return &LedisCache{
		ldb:     ldb,
		db:      db,
		dataDir: dataDir,
	}, nil
}

// Get 获取值
func (c *LedisCache) Get(key string) (interface{}, bool) {
	data, err := c.db.Get([]byte(key))
	if err != nil || data == nil {
		return nil, false
	}

	// 尝试反序列化
	var value interface{}
	if err := jsonUnmarshal(data, &value); err != nil {
		return string(data), true
	}
	return value, true
}

// Set 设置值（带过期时间）
func (c *LedisCache) Set(key string, value interface{}, expiration time.Duration) {
	data, err := c.serialize(value)
	if err != nil {
		return
	}

	if expiration > 0 {
		seconds := int64(expiration.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		c.db.SetEX([]byte(key), seconds, data)
	} else {
		c.db.Set([]byte(key), data)
	}
}

// Delete 删除键
func (c *LedisCache) Delete(key string) {
	c.db.Del([]byte(key))
}

// Flush 清空数据库
func (c *LedisCache) Flush() {
	c.ldb.FlushAll()
}

// ItemCount 获取键数量（LedisDB 不直接支持，返回估算值）
func (c *LedisCache) ItemCount() int {
	// LedisDB 没有直接的键计数命令，返回 0 表示未知
	return 0
}

// Close 关闭连接
func (c *LedisCache) Close() error {
	if c.ldb != nil {
		c.ldb.Close()
	}
	return nil
}

// serialize 序列化值
func (c *LedisCache) serialize(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return jsonMarshal(value)
	}
}

// jsonMarshal 序列化为 JSON
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal 反序列化 JSON
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
