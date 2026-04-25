package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"openaide/backend/src/models"
	"openaide/backend/src/services"
	"github.com/gin-gonic/gin"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *services.AuthService
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6,max=100"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	User         *models.User      `json:"user"`
	AccessToken  string            `json:"access_token"`
	RefreshToken string            `json:"refresh_token"`
	ExpiresIn    int64             `json:"expires_in"` // 秒
	Session      *models.UserSession `json:"session"`
}

// RefreshRequest 刷新请求
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=100"`
}

// CreateAPIKeyRequest 创建 API Key 请求
type CreateAPIKeyRequest struct {
	Name        string   `json:"name" binding:"required,max=100"`
	Permissions []string `json:"permissions"`
	ExpiresIn   int      `json:"expires_in"` // 过期时间（小时），0 表示永不过期
}

// APIKeyResponse API Key 响应
type APIKeyResponse struct {
	*models.APIKey
	Key string `json:"key"` // 完整的 key，只在创建时显示一次
}

// Register 用户注册
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.authService.Register(req.Username, req.Email, req.Password)
	if err != nil {
		if err == services.ErrUserExists {
			c.JSON(http.StatusConflict, gin.H{"error": "用户名或邮箱已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "注册成功",
		"user":    user,
	})
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	user, session, refreshToken, err := h.authService.Login(req.Username, req.Password, ipAddress, userAgent)
	if err != nil {
		switch err {
		case services.ErrUserNotFound:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		case services.ErrInvalidPassword:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		case services.ErrUserInactive:
			c.JSON(http.StatusForbidden, gin.H{"error": "账户已被禁用"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 生成 token
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		User:         user,
		AccessToken:  token,
		RefreshToken: refreshToken,
		ExpiresIn:    24 * 3600, // 24小时
		Session:      session,
	})
}

// Logout 用户登出
func (h *AuthHandler) Logout(c *gin.Context) {
	sessionID, _ := c.Get("session_id")
	if sessionID != nil {
		h.authService.Logout(sessionID.(string))
	}

	// 将当前 token 加入黑名单
	token := c.GetHeader("Authorization")
	if token != "" {
		token = strings.TrimPrefix(token, "Bearer ")
		h.authService.BlacklistToken(token, 24*time.Hour)
	}

	c.JSON(http.StatusOK, gin.H{"message": "登出成功"})
}

// RefreshToken 刷新 Token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, token, refreshToken, err := h.authService.RefreshToken(req.RefreshToken)
	if err != nil {
		switch err {
		case services.ErrInvalidToken:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的刷新令牌"})
		case services.ErrTokenExpired:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "刷新令牌已过期，请重新登录"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  token,
		"refresh_token": refreshToken,
		"expires_in":    24 * 3600,
		"user":          user,
	})
}

// GetProfile 获取当前用户信息
func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	user, err := h.authService.GetUser(userIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateProfile 更新当前用户信息
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.authService.UpdateUser(userIDStr, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// ChangePassword 修改密码
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.authService.ChangePassword(userIDStr, req.OldPassword, req.NewPassword)
	if err != nil {
		switch err {
		case services.ErrInvalidPassword:
			c.JSON(http.StatusBadRequest, gin.H{"error": "原密码错误"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功，请重新登录"})
}

// GetSessions 获取当前用户的会话列表
func (h *AuthHandler) GetSessions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	sessions, err := h.authService.GetSessions(userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, sessions)
}

// LogoutSession 登出指定会话
func (h *AuthHandler) LogoutSession(c *gin.Context) {
	sessionID := c.Param("id")
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	// 验证会话属于当前用户
	sessions, _ := h.authService.GetSessions(userIDStr)
	isOwner := false
	for _, s := range sessions {
		if s.ID == sessionID {
			isOwner = true
			break
		}
	}

	if !isOwner {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	if err := h.authService.Logout(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "会话已登出"})
}

// ListAPIKeys 列出用户的 API Keys
func (h *AuthHandler) ListAPIKeys(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	keys, err := h.authService.ListAPIKeys(userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, keys)
}

// CreateAPIKey 创建 API Key
func (h *AuthHandler) CreateAPIKey(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的用户ID"})
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Hour)
		expiresAt = &t
	}

	apiKey, keyString, err := h.authService.CreateAPIKey(userIDStr, req.Name, req.Permissions, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, APIKeyResponse{
		APIKey: apiKey,
		Key:    keyString,
	})
}

// RevokeAPIKey 撤销 API Key
func (h *AuthHandler) RevokeAPIKey(c *gin.Context) {
	keyID := c.Param("id")

	if err := h.authService.RevokeAPIKey(keyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API Key 已撤销"})
}

// GetPermissions 获取权限列表
func (h *AuthHandler) GetPermissions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"permissions": models.SystemPermissions,
		"roles":       models.SystemRoles,
	})
}

// ============ 管理员接口 ============

// ListUsers 列出用户（管理员）
func (h *AuthHandler) ListUsers(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"))
	pageSize := parseInt(c.DefaultQuery("page_size", "20"))

	users, total, err := h.authService.ListUsers(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// GetUser 获取用户（管理员）
func (h *AuthHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.authService.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser 更新用户（管理员）
func (h *AuthHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.authService.UpdateUser(userID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser 删除用户（管理员）
func (h *AuthHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.authService.DeleteUser(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}

// ============ 认证中间件 ============

// AuthMiddleware 认证中间件
func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 本地开发模式：检查环境变量
		if os.Getenv("OPENAIDE_LOCAL_MODE") == "true" {
			c.Set("user_id", "local-user")
			c.Set("username", "local-user")
			c.Set("role", "admin")
			c.Set("permissions", []string{"*"})
			c.Next()
			return
		}

		// 1. 尝试从 Header 获取 Bearer Token
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != authHeader {
				claims, err := h.authService.ValidateToken(token)
				if err == nil {
					c.Set("user_id", claims.UserID)
					c.Set("username", claims.Username)
					c.Set("role", claims.Role)
					c.Set("permissions", claims.Permissions)
					c.Next()
					return
				}
			}
		}

		// 2. 尝试从 Query 获取 Token
		token := c.Query("token")
		if token != "" {
			claims, err := h.authService.ValidateToken(token)
			if err == nil {
				c.Set("user_id", claims.UserID)
				c.Set("username", claims.Username)
				c.Set("role", claims.Role)
				c.Set("permissions", claims.Permissions)
				c.Next()
				return
			}
		}

		// 3. 尝试从 Header 获取 API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			key, user, err := h.authService.ValidateAPIKey(apiKey)
			if err == nil {
				c.Set("user_id", user.ID)
				c.Set("username", user.Username)
				c.Set("role", user.Role)
				c.Set("permissions", key.Permissions)
				c.Set("api_key_id", key.ID)
				c.Next()
				return
			}
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权访问"})
		c.Abort()
	}
}

// OptionalAuth 可选认证中间件
func (h *AuthHandler) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token != authHeader {
				claims, err := h.authService.ValidateToken(token)
				if err == nil {
					c.Set("user_id", claims.UserID)
					c.Set("username", claims.Username)
					c.Set("role", claims.Role)
					c.Set("permissions", claims.Permissions)
				}
			}
		}
		c.Next()
	}
}

// AdminRequired 管理员权限中间件
func (h *AuthHandler) AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// PermissionRequired 权限检查中间件
func (h *AuthHandler) PermissionRequired(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			c.Next()
			return
		}

		permissions, exists := c.Get("permissions")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			c.Abort()
			return
		}

		perms := permissions.([]string)
		hasPermission := false
		for _, p := range perms {
			if p == "*" || p == permission {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// 辅助函数
func parseInt(s string) int {
	result, _ := strconv.Atoi(s)
	if result < 0 {
		result = 0
	}
	return result
}
