package services

import (
	"gorm.io/gorm"
)

// BaseService 服务基类，提取公共依赖
// 使用 Go 嵌入结构体，减少服务间重复声明
type BaseService struct {
	DB     *gorm.DB
	Logger *LoggerService
	Cache  *CacheService
}
