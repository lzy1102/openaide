package storage

import (
	"fmt"
	"log"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DBConfig 数据库配置
type DBConfig struct {
	Type     string `json:"type"`      // "sqlite", "postgres", "mysql"
	DSN      string `json:"dsn"`       // 连接字符串
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
	SSLMode  string `json:"ssl_mode,omitempty"` // postgres only
}

// NewDB 根据配置创建数据库连接
func NewDB(cfg DBConfig) (*gorm.DB, error) {
	switch cfg.Type {
	case "sqlite", "":
		return newSQLite(cfg)
	case "postgres":
		return newPostgres(cfg)
	case "mysql":
		return newMySQL(cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

// newSQLite SQLite 连接
func newSQLite(cfg DBConfig) (*gorm.DB, error) {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = "openaide.db"
	}
	log.Printf("Connecting to SQLite: %s", dsn)
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{})
}

// newPostgres PostgreSQL 连接
func newPostgres(cfg DBConfig) (*gorm.DB, error) {
	dsn := cfg.DSN
	if dsn == "" {
		sslMode := cfg.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		port := cfg.Port
		if port == 0 {
			port = 5432
		}
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			cfg.Host, port, cfg.User, cfg.Password, cfg.Database, sslMode)
	}
	log.Printf("Connecting to PostgreSQL: %s", maskDSN(dsn))
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

// newMySQL MySQL 连接
func newMySQL(cfg DBConfig) (*gorm.DB, error) {
	dsn := cfg.DSN
	if dsn == "" {
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, port, cfg.Database)
	}
	log.Printf("Connecting to MySQL: %s", maskDSN(dsn))
	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

// maskDSN 隐藏密码
func maskDSN(dsn string) string {
	// 简单替换 password=xxx
	if idx := strings.Index(dsn, "password="); idx != -1 {
		end := idx + 9
		for end < len(dsn) && dsn[end] != ' ' {
			end++
		}
		return dsn[:idx+9] + "***" + dsn[end:]
	}
	return dsn
}
