package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"openaide/backend/core/actor"
)

// Claims JWT 声明
type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"uname"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

// Service 认证服务
type Service struct {
	secret []byte
	users  *actor.SafeMap[string, *User]
}

// User 用户
type User struct {
	Username string `json:"username"`
	Password string `json:"password"` // SHA256 hash
	Role     string `json:"role"`     // admin, user
}

// NewService 创建认证服务
func NewService(secret string) *Service {
	if secret == "" {
		secret = "openaide-default-secret-change-in-production"
	}
	s := &Service{
		secret: []byte(secret),
		users:  actor.NewSafeMap[string, *User](8),
	}
	// 默认 admin 用户
	s.users.Store("admin", &User{
		Username: "admin",
		Password: hashPassword("admin123"),
		Role:     "admin",
	})
	return s
}

// Register 注册用户
func (s *Service) Register(username, password string) (*User, error) {
	if _, exists := s.users.Load(username); exists {
		return nil, fmt.Errorf("user already exists: %s", username)
	}
	if len(password) < 6 {
		return nil, fmt.Errorf("password too short (min 6 chars)")
	}

	user := &User{
		Username: username,
		Password: hashPassword(password),
		Role:     "user",
	}
	s.users.Store(username, user)
	return user, nil
}

// Login 登录，返回 JWT token
func (s *Service) Login(username, password string) (string, error) {
	user, ok := s.users.Load(username)

	if !ok || user.Password != hashPassword(password) {
		return "", fmt.Errorf("invalid username or password")
	}

	return s.issueToken(user)
}

// issueToken 签发 JWT
func (s *Service) issueToken(user *User) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	claims := Claims{
		UserID:   user.Username,
		Username: user.Username,
		Role:     user.Role,
		Exp:      time.Now().Add(24 * time.Hour).Unix(),
	}
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)

	signingInput := header + "." + payloadB64
	sig := sign([]byte(signingInput), s.secret)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64, nil
}

// Verify 验证 JWT，返回 Claims
func (s *Service) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	actualSig := sign([]byte(signingInput), s.secret)
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload encoding")
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}

	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// Middleware JWT 认证中间件
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 白名单路由
		if r.URL.Path == "/health" ||
			r.URL.Path == "/api/v1/auth/register" ||
			r.URL.Path == "/api/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := s.Verify(token)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
			return
		}

		// 注入用户信息到 header
		r.Header.Set("X-User-ID", claims.UserID)
		r.Header.Set("X-User-Role", claims.Role)
		next.ServeHTTP(w, r)
	})
}

// AuthHandler 认证相关的 HTTP handler
func (s *Service) AuthHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v1/auth/register" && r.Method == "POST":
		s.handleRegister(w, r)
	case r.URL.Path == "/api/v1/auth/login" && r.Method == "POST":
		s.handleLogin(w, r)
	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	user, err := s.Register(req.Username, req.Password)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]interface{}{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	token, err := s.Login(req.Username, req.Password)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   "86400",
	})
}

// ============ helpers ============

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return fmt.Sprintf("%x", h)
}

func sign(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
