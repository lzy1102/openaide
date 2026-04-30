package storage

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"openaide/backend/src/logger"

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

	// 连接池配置
	MaxOpenConns    int           `json:"max_open_conns,omitempty"`    // 最大打开连接数，默认 100
	MaxIdleConns    int           `json:"max_idle_conns,omitempty"`    // 最大空闲连接数，默认 10
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime,omitempty"` // 连接最大生命周期，默认 1h
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time,omitempty"` // 连接最大空闲时间，默认 10m
}

// NewDB 根据配置创建数据库连接
func NewDB(cfg DBConfig) (*gorm.DB, error) {
	var db *gorm.DB
	var err error

	switch cfg.Type {
	case "sqlite", "":
		db, err = newSQLite(cfg)
	case "postgres":
		db, err = newPostgres(cfg)
	case "mysql":
		db, err = newMySQL(cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	if err != nil {
		return nil, err
	}

	// 配置连接池
	if err := configurePool(db, cfg); err != nil {
		return nil, fmt.Errorf("failed to configure connection pool: %w", err)
	}

	return db, nil
}

// configurePool 配置数据库连接池
func configurePool(db *gorm.DB, cfg DBConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// 最大打开连接数
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 100
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	// 最大空闲连接数
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	// 连接最大生命周期
	maxLifetime := cfg.ConnMaxLifetime
	if maxLifetime <= 0 {
		maxLifetime = time.Hour
	}
	sqlDB.SetConnMaxLifetime(maxLifetime)

	// 连接最大空闲时间
	maxIdleTime := cfg.ConnMaxIdleTime
	if maxIdleTime <= 0 {
		maxIdleTime = 10 * time.Minute
	}
	sqlDB.SetConnMaxIdleTime(maxIdleTime)

	logger.WithComponent("DB").Info("connection pool configured",
		"max_open", maxOpen, "max_idle", maxIdle, "max_lifetime", maxLifetime, "max_idle_time", maxIdleTime)

	return nil
}

// newSQLite SQLite 连接
func newSQLite(cfg DBConfig) (*gorm.DB, error) {
	uri := cfg.URI
	if uri == "" {
		uri = "openaide.db"
	}
	logger.WithComponent("DB").Info("Connecting to SQLite", "uri", uri)
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
	logger.WithComponent("DB").Info("connecting to PostgreSQL", "uri", maskURI(uri))
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
	logger.WithComponent("DB").Info("connecting to MySQL", "uri", maskURI(uri))
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
