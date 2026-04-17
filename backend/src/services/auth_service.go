package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"openaide/backend/src/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 认证相关错误
var (
	ErrUserNotFound     = errors.New("user not found")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrUserExists       = errors.New("user already exists")
	ErrUserInactive     = errors.New("user is inactive")
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidAPIKey    = errors.New("invalid api key")
	ErrAPIKeyExpired    = errors.New("api key expired")
	ErrAPIKeyRevoked    = errors.New("api key revoked")
	ErrPermissionDenied  = errors.New("permission denied")
)

// JWTClaims JWT 声明
type JWTClaims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// AuthConfig 认证配置
type AuthConfig struct {
	JWTSecret          string
	JWTExpiry          time.Duration
	RefreshTokenExpiry time.Duration
	Issuer             string
}

// AuthService 认证服务
type AuthService struct {
	db     *gorm.DB
	config *AuthConfig
	cache  *CacheService
}

// NewAuthService 创建认证服务
func NewAuthService(db *gorm.DB, cache *CacheService, config *AuthConfig) *AuthService {
	if config == nil {
		config = &AuthConfig{
			JWTSecret:          "your-secret-key-change-in-production",
			JWTExpiry:          24 * time.Hour,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
			Issuer:             "openaide",
		}
	}
	return &AuthService{
		db:     db,
		config: config,
		cache:  cache,
	}
}

// Register 用户注册
func (s *AuthService) Register(username, email, password string) (*models.User, error) {
	// 检查用户是否存在
	var existingUser models.User
	if err := s.db.Where("username = ? OR email = ?", username, email).First(&existingUser).Error; err == nil {
		return nil, ErrUserExists
	}

	// 创建用户
	user := &models.User{
		Username: username,
		Email:    email,
		Role:     "user",
		Status:   "active",
	}

	if err := user.SetPassword(password); err != nil {
		return nil, fmt.Errorf("failed to set password: %w", err)
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// Login 用户登录
func (s *AuthService) Login(username, password, ipAddress, userAgent string) (*models.User, *models.UserSession, string, error) {
	// 查找用户
	var user models.User
	if err := s.db.Where("username = ? OR email = ?", username, username).First(&user).Error; err != nil {
		return nil, nil, "", ErrUserNotFound
	}

	// 检查用户状态
	if !user.IsActive() {
		return nil, nil, "", ErrUserInactive
	}

	// 验证密码
	if !user.CheckPassword(password) {
		return nil, nil, "", ErrInvalidPassword
	}

	// 更新登录信息
	now := time.Now()
	user.LastLoginAt = &now
	user.LastLoginIP = ipAddress
	user.LoginCount++
	s.db.Save(&user)

	// 创建会话
	session, refreshToken, err := s.createSession(&user, ipAddress, userAgent)
	if err != nil {
		return nil, nil, "", err
	}

	return &user, session, refreshToken, nil
}

// Logout 用户登出
func (s *AuthService) Logout(sessionID string) error {
	return s.db.Delete(&models.UserSession{}, "id = ?", sessionID).Error
}

// LogoutAll 登出所有设备
func (s *AuthService) LogoutAll(userID string) error {
	return s.db.Delete(&models.UserSession{}, "user_id = ?", userID).Error
}

// GenerateToken 生成 JWT Token
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	// 获取用户权限
	permissions := s.getUserPermissions(user.Role)

	claims := JWTClaims{
		UserID:      user.ID,
		Username:    user.Username,
		Role:        user.Role,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.config.JWTExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    s.config.Issuer,
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWTSecret))
}

// ValidateToken 验证 JWT Token
func (s *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	// 检查黑名单
	if s.isTokenBlacklisted(tokenString) {
		return nil, ErrInvalidToken
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshToken 刷新 Token
func (s *AuthService) RefreshToken(refreshToken string) (*models.User, string, string, error) {
	// 查找会话
	var session models.UserSession
	if err := s.db.Where("refresh_token = ?", refreshToken).First(&session).Error; err != nil {
		return nil, "", "", ErrInvalidToken
	}

	// 检查是否过期
	if session.IsExpired() {
		s.db.Delete(&session)
		return nil, "", "", ErrTokenExpired
	}

	// 获取用户
	var user models.User
	if err := s.db.First(&user, session.UserID).Error; err != nil {
		return nil, "", "", ErrUserNotFound
	}

	// 检查用户状态
	if !user.IsActive() {
		return nil, "", "", ErrUserInactive
	}

	// 生成新 token
	newToken, err := s.GenerateToken(&user)
	if err != nil {
		return nil, "", "", err
	}

	// 生成新的 refresh token
	newRefreshToken := uuid.New().String()
	session.RefreshToken = newRefreshToken
	session.LastActiveAt = time.Now()
	s.db.Save(&session)

	return &user, newToken, newRefreshToken, nil
}

// ValidateAPIKey 验证 API Key
func (s *AuthService) ValidateAPIKey(keyString string) (*models.APIKey, *models.User, error) {
	// 解析 key (格式: prefix_secret)
	prefix := ""
	if len(keyString) >= 8 {
		prefix = keyString[:8]
	}

	// 查找 API Key
	var apiKey models.APIKey
	if err := s.db.Where("prefix = ?", prefix).First(&apiKey).Error; err != nil {
		return nil, nil, ErrInvalidAPIKey
	}

	// 验证 secret
	hash := sha256.Sum256([]byte(keyString))
	if apiKey.Secret != hex.EncodeToString(hash[:]) {
		return nil, nil, ErrInvalidAPIKey
	}

	// 检查状态
	if apiKey.Status == "revoked" {
		return nil, nil, ErrAPIKeyRevoked
	}

	// 检查过期
	if apiKey.IsExpired() {
		return nil, nil, ErrAPIKeyExpired
	}

	// 获取用户
	var user models.User
	if err := s.db.First(&user, apiKey.UserID).Error; err != nil {
		return nil, nil, ErrUserNotFound
	}

	// 更新使用信息
	now := time.Now()
	apiKey.LastUsedAt = &now
	apiKey.UsageCount++
	s.db.Save(&apiKey)

	return &apiKey, &user, nil
}

// CreateAPIKey 创建 API Key
func (s *AuthService) CreateAPIKey(userID, name string, permissions []string, expiresAt *time.Time) (*models.APIKey, string, error) {
	// 生成密钥
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, "", fmt.Errorf("failed to generate key: %w", err)
	}
	keyString := "sk-" + hex.EncodeToString(keyBytes)

	// 计算前缀和哈希
	prefix := keyString[:8]
	hash := sha256.Sum256([]byte(keyString))
	secret := hex.EncodeToString(hash[:])

	apiKey := &models.APIKey{
		UserID:      userID,
		Name:        name,
		Key:         keyString[:16] + "..." + keyString[len(keyString)-4:],
		Prefix:      prefix,
		Secret:      secret,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		Status:      "active",
	}

	if err := s.db.Create(apiKey).Error; err != nil {
		return nil, "", fmt.Errorf("failed to create api key: %w", err)
	}

	return apiKey, keyString, nil
}

// RevokeAPIKey 撤销 API Key
func (s *AuthService) RevokeAPIKey(keyID string) error {
	return s.db.Model(&models.APIKey{}).Where("id = ?", keyID).Update("status", "revoked").Error
}

// ListAPIKeys 列出用户的 API Keys
func (s *AuthService) ListAPIKeys(userID string) ([]models.APIKey, error) {
	var keys []models.APIKey
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// GetUser 获取用户
func (s *AuthService) GetUser(userID string) (*models.User, error) {
	var user models.User
	err := s.db.First(&user, "id = ?", userID).Error
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

// UpdateUser 更新用户
func (s *AuthService) UpdateUser(userID string, updates map[string]interface{}) (*models.User, error) {
	user, err := s.GetUser(userID)
	if err != nil {
		return nil, err
	}

	// 不允许直接更新的字段
	delete(updates, "id")
	delete(updates, "password_hash")
	delete(updates, "created_at")

	if err := s.db.Model(user).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return s.GetUser(userID)
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(userID, oldPassword, newPassword string) error {
	user, err := s.GetUser(userID)
	if err != nil {
		return err
	}

	// 验证旧密码
	if !user.CheckPassword(oldPassword) {
		return ErrInvalidPassword
	}

	// 设置新密码
	if err := user.SetPassword(newPassword); err != nil {
		return fmt.Errorf("failed to set password: %w", err)
	}

	// 保存
	if err := s.db.Save(user).Error; err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}

	// 撤销所有会话（强制重新登录）
	s.LogoutAll(userID)

	return nil
}

// ListUsers 列出用户（管理员）
func (s *AuthService) ListUsers(page, pageSize int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	offset := (page - 1) * pageSize

	s.db.Model(&models.User{}).Count(&total)
	err := s.db.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&users).Error

	return users, total, err
}

// DeleteUser 删除用户
func (s *AuthService) DeleteUser(userID string) error {
	// 软删除
	return s.db.Delete(&models.User{}, "id = ?", userID).Error
}

// CheckPermission 检查权限
func (s *AuthService) CheckPermission(userID, permission string) bool {
	user, err := s.GetUser(userID)
	if err != nil {
		return false
	}

	// 管理员拥有所有权限
	if user.IsAdmin() {
		return true
	}

	// 获取角色权限
	permissions := s.getUserPermissions(user.Role)
	for _, p := range permissions {
		if p == "*" || p == permission {
			return true
		}
	}

	return false
}

// GetSessions 获取用户会话列表
func (s *AuthService) GetSessions(userID string) ([]models.UserSession, error) {
	var sessions []models.UserSession
	err := s.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("last_active_at DESC").
		Find(&sessions).Error
	return sessions, err
}

// createSession 创建会话
func (s *AuthService) createSession(user *models.User, ipAddress, userAgent string) (*models.UserSession, string, error) {
	refreshToken := uuid.New().String()
	expiresAt := time.Now().Add(s.config.RefreshTokenExpiry)

	// 解析设备类型
	deviceType := "web"
	if len(userAgent) > 0 {
		if containsSubstring(userAgent, "Mobile") || containsSubstring(userAgent, "Android") || containsSubstring(userAgent, "iPhone") {
			deviceType = "mobile"
		} else if containsSubstring(userAgent, "Electron") {
			deviceType = "desktop"
		}
	}

	session := &models.UserSession{
		UserID:       user.ID,
		RefreshToken: refreshToken,
		DeviceType:   deviceType,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		ExpiresAt:    expiresAt,
	}

	if err := s.db.Create(session).Error; err != nil {
		return nil, "", fmt.Errorf("failed to create session: %w", err)
	}

	return session, refreshToken, nil
}

// getUserPermissions 获取用户权限
func (s *AuthService) getUserPermissions(roleName string) []string {
	for _, role := range models.SystemRoles {
		if role.Name == roleName {
			return role.Permissions
		}
	}
	return []string{}
}

// isTokenBlacklisted 检查 Token 是否在黑名单
func (s *AuthService) isTokenBlacklisted(token string) bool {
	if s.cache == nil {
		return false
	}
	key := "token:blacklist:" + token
	_, found := s.cache.Get(key)
	return found
}

// BlacklistToken 将 Token 加入黑名单
func (s *AuthService) BlacklistToken(token string, expiry time.Duration) error {
	if s.cache == nil {
		return nil
	}
	key := "token:blacklist:" + token
	s.cache.Set(key, true, expiry)
	return nil
}

// 辅助函数
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
