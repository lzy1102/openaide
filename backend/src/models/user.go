package models

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID            string         `json:"id" gorm:"primaryKey"`
	Username      string         `json:"username" gorm:"uniqueIndex;size:50;not null"`
	Email         string         `json:"email" gorm:"uniqueIndex;size:100"`
	PasswordHash  string         `json:"-" gorm:"size:255"`
	DisplayName   string         `json:"display_name" gorm:"size:100"`
	Avatar        string         `json:"avatar" gorm:"size:500"`
	Role          string         `json:"role" gorm:"size:20;default:'user'"` // admin, user, guest
	Status        string         `json:"status" gorm:"size:20;default:'active'"` // active, inactive, banned
	LastLoginAt   *time.Time     `json:"last_login_at"`
	LastLoginIP   string         `json:"last_login_ip" gorm:"size:45"`
	LoginCount    int            `json:"login_count" gorm:"default:0"`
	Preferences   JSONMap `json:"preferences" gorm:"type:json"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	APIKeys       []APIKey       `json:"api_keys,omitempty" gorm:"foreignKey:UserID"`
	Sessions      []UserSession  `json:"sessions,omitempty" gorm:"foreignKey:UserID"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// BeforeCreate 创建前钩子
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate 更新前钩子
func (u *User) BeforeUpdate(tx *gorm.DB) error {
	u.UpdatedAt = time.Now()
	return nil
}

// SetPassword 设置密码
func (u *User) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword 验证密码
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// IsAdmin 是否是管理员
func (u *User) IsAdmin() bool {
	return u.Role == "admin"
}

// IsActive 是否活跃
func (u *User) IsActive() bool {
	return u.Status == "active"
}

// APIKey API 密钥模型
type APIKey struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	UserID      string         `json:"user_id" gorm:"index;not null"`
	Name        string         `json:"name" gorm:"size:100;not null"`
	Key         string         `json:"key" gorm:"uniqueIndex;size:64;not null"`
	Prefix      string         `json:"prefix" gorm:"size:8"` // 密钥前缀，用于显示
	Secret      string         `json:"-" gorm:"size:64"`     // 密钥密文（用于验证）
	Permissions JSONSlice `json:"permissions" gorm:"type:json"` // 权限列表
	RateLimit   int            `json:"rate_limit" gorm:"default:100"` // 每分钟请求限制
	ExpiresAt   *time.Time     `json:"expires_at"`
	LastUsedAt  *time.Time     `json:"last_used_at"`
	UsageCount  int64          `json:"usage_count" gorm:"default:0"`
	Status      string         `json:"status" gorm:"size:20;default:'active'"` // active, revoked
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`

	// 关联
	User        *User          `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName 指定表名
func (APIKey) TableName() string {
	return "api_keys"
}

// BeforeCreate 创建前钩子
func (k *APIKey) BeforeCreate(tx *gorm.DB) error {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	k.CreatedAt = time.Now()
	k.UpdatedAt = time.Now()
	return nil
}

// IsExpired 是否过期
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsValid 是否有效
func (k *APIKey) IsValid() bool {
	return k.Status == "active" && !k.IsExpired()
}

// HasPermission 检查权限
func (k *APIKey) HasPermission(permission string) bool {
	for _, p := range k.Permissions {
		if p == "*" || p == permission {
			return true
		}
	}
	return false
}

// UserSession 用户会话模型
type UserSession struct {
	ID           string     `json:"id" gorm:"primaryKey"`
	UserID       string     `json:"user_id" gorm:"index;not null"`
	Token        string     `json:"-" gorm:"uniqueIndex;size:500"`
	RefreshToken string     `json:"-" gorm:"size:500"`
	DeviceType   string     `json:"device_type" gorm:"size:50"`  // web, mobile, desktop
	DeviceName   string     `json:"device_name" gorm:"size:100"`
	UserAgent    string     `json:"user_agent" gorm:"size:500"`
	IPAddress    string     `json:"ip_address" gorm:"size:45"`
	Location     string     `json:"location" gorm:"size:100"`
	ExpiresAt    time.Time  `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	LastActiveAt time.Time  `json:"last_active_at"`

	// 关联
	User         *User      `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

// TableName 指定表名
func (UserSession) TableName() string {
	return "user_sessions"
}

// BeforeCreate 创建前钩子
func (s *UserSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	s.CreatedAt = time.Now()
	s.LastActiveAt = time.Now()
	return nil
}

// IsExpired 是否过期
func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// Role 角色模型
type Role struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"uniqueIndex;size:50;not null"`
	Description string         `json:"description" gorm:"size:255"`
	Permissions JSONSlice `json:"permissions" gorm:"type:json"`
	IsSystem    bool           `json:"is_system" gorm:"default:false"` // 系统角色不可删除
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "roles"
}

// BeforeCreate 创建前钩子
func (r *Role) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	r.CreatedAt = time.Now()
	r.UpdatedAt = time.Now()
	return nil
}

// HasPermission 检查权限
func (r *Role) HasPermission(permission string) bool {
	for _, p := range r.Permissions {
		if p == "*" || p == permission {
			return true
		}
	}
	return false
}

// 系统预定义角色
var SystemRoles = []Role{
	{
		Name:        "admin",
		Description: "系统管理员，拥有所有权限",
		Permissions: []string{"*"},
		IsSystem:    true,
	},
	{
		Name:        "user",
		Description: "普通用户，拥有基本权限",
		Permissions: []string{
			"dialogue:read", "dialogue:write",
			"model:read", "model:execute",
			"task:read", "task:write",
			"knowledge:read", "knowledge:write",
			"workflow:read", "workflow:execute",
		},
		IsSystem: true,
	},
	{
		Name:        "guest",
		Description: "访客，只读权限",
		Permissions: []string{
			"dialogue:read",
			"model:read",
			"knowledge:read",
		},
		IsSystem: true,
	},
}

// Permission 权限定义
type Permission struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// 系统权限列表
var SystemPermissions = []Permission{
	// 对话权限
	{Name: "dialogue:read", Description: "查看对话", Category: "dialogue"},
	{Name: "dialogue:write", Description: "创建/编辑对话", Category: "dialogue"},
	{Name: "dialogue:delete", Description: "删除对话", Category: "dialogue"},

	// 模型权限
	{Name: "model:read", Description: "查看模型", Category: "model"},
	{Name: "model:write", Description: "创建/编辑模型", Category: "model"},
	{Name: "model:delete", Description: "删除模型", Category: "model"},
	{Name: "model:execute", Description: "执行模型", Category: "model"},

	// 任务权限
	{Name: "task:read", Description: "查看任务", Category: "task"},
	{Name: "task:write", Description: "创建/编辑任务", Category: "task"},
	{Name: "task:delete", Description: "删除任务", Category: "task"},

	// 知识库权限
	{Name: "knowledge:read", Description: "查看知识库", Category: "knowledge"},
	{Name: "knowledge:write", Description: "创建/编辑知识", Category: "knowledge"},
	{Name: "knowledge:delete", Description: "删除知识", Category: "knowledge"},

	// 工作流权限
	{Name: "workflow:read", Description: "查看工作流", Category: "workflow"},
	{Name: "workflow:write", Description: "创建/编辑工作流", Category: "workflow"},
	{Name: "workflow:delete", Description: "删除工作流", Category: "workflow"},
	{Name: "workflow:execute", Description: "执行工作流", Category: "workflow"},

	// 用户管理权限
	{Name: "user:read", Description: "查看用户", Category: "user"},
	{Name: "user:write", Description: "创建/编辑用户", Category: "user"},
	{Name: "user:delete", Description: "删除用户", Category: "user"},

	// 系统管理权限
	{Name: "system:config", Description: "系统配置", Category: "system"},
	{Name: "system:logs", Description: "查看日志", Category: "system"},
	{Name: "system:monitor", Description: "系统监控", Category: "system"},
}
