package storage

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DBConfig 数据库配置
type DBConfig struct {
	Type string `json:"type"` // "sqlite", "postgres", "mysql"
	URI  string `json:"uri"`  // 连接 URI/DSN（优先使用）
	// 以下为分开配置（当 URI 为空时使用）
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
	uri := cfg.URI
	if uri == "" {
		uri = "openaide.db"
	}
	log.Printf("Connecting to SQLite: %s", uri)
	return gorm.Open(sqlite.Open(uri), &gorm.Config{})
}

// newPostgres PostgreSQL 连接
func newPostgres(cfg DBConfig) (*gorm.DB, error) {
	uri := cfg.URI
	if uri == "" {
		// 构建 PostgreSQL URI: postgres://user:password@host:port/db?sslmode=disable
		sslMode := cfg.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		port := cfg.Port
		if port == 0 {
			port = 5432
		}
		uri = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			cfg.User, cfg.Password, cfg.Host, port, cfg.Database, sslMode)
	}
	log.Printf("Connecting to PostgreSQL: %s", maskURI(uri))
	return gorm.Open(postgres.Open(uri), &gorm.Config{})
}

// newMySQL MySQL 连接
func newMySQL(cfg DBConfig) (*gorm.DB, error) {
	uri := cfg.URI
	if uri == "" {
		// 构建 MySQL URI: user:password@tcp(host:port)/db?charset=utf8mb4&parseTime=True&loc=Local
		port := cfg.Port
		if port == 0 {
			port = 3306
		}
		uri = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.User, cfg.Password, cfg.Host, port, cfg.Database)
	}
	log.Printf("Connecting to MySQL: %s", maskURI(uri))
	return gorm.Open(mysql.Open(uri), &gorm.Config{})
}

// maskURI 隐藏密码
func maskURI(uri string) string {
	// 尝试解析 URI
	if u, err := url.Parse(uri); err == nil && u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), "***")
			return u.String()
		}
	}
	// 简单替换 password=xxx 或 :password@
	masked := uri
	if idx := strings.Index(masked, "password="); idx != -1 {
		end := idx + 9
		for end < len(masked) && masked[end] != ' ' && masked[end] != '&' {
			end++
		}
		masked = masked[:idx+9] + "***" + masked[end:]
	}
	return masked
}
