package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/internal/database"
)

type Session struct {
	Token     string
	UserID    uint
	Username  string
	Email     string
	Role      string
	CreatedAt time.Time
}

var (
	sessions = make(map[string]Session)
	sessionMu sync.RWMutex
)

func HashPassword(password string) string {
	h := sha256.New()
	h.Write([]byte("kenerateai_salt_" + password))
	return hex.EncodeToString(h.Sum(nil))
}

func VerifyPassword(password, hash string) bool {
	return HashPassword(password) == hash
}

func GenerateToken(userID uint, username, role string) string {
	tokenData := fmt.Sprintf("%d-%s-%s-%d", userID, username, role, time.Now().UnixNano())
	h := sha256.New()
	h.Write([]byte(tokenData))
	token := hex.EncodeToString(h.Sum(nil))

	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessions[token] = Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
	}

	return token
}

func GetSession(token string) (Session, bool) {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	sess, ok := sessions[token]
	return sess, ok
}

func RevokeToken(token string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessions, token)
}

func SeedDefaultUsers(db *gorm.DB) error {
	var count int64
	db.Model(&database.User{}).Count(&count)
	if count > 0 {
		return nil
	}

	// Create default Dev user
	devUser := database.User{
		Username:         "dev_admin",
		Email:            "dev@kenerateai.com",
		PasswordHash:     HashPassword("admin123"),
		Role:             "dev",
		TasksCompleted:   0,
		TasksPending:     0,
		VerificationRate: 100.0,
		CreatedAt:        time.Now(),
	}
	if err := db.Create(&devUser).Error; err != nil {
		return err
	}

	// Create default Intern users
	interns := []database.User{
		{
			Username:         "alex_intern",
			Email:            "alex@kenerateai.com",
			PasswordHash:     HashPassword("intern123"),
			Role:             "intern",
			TasksCompleted:   12,
			TasksPending:     2,
			VerificationRate: 92.5,
			CreatedAt:        time.Now(),
		},
		{
			Username:         "maya_intern",
			Email:            "maya@kenerateai.com",
			PasswordHash:     HashPassword("intern123"),
			Role:             "intern",
			TasksCompleted:   8,
			TasksPending:     1,
			VerificationRate: 100.0,
			CreatedAt:        time.Now(),
		},
		{
			Username:         "sam_intern",
			Email:            "sam@kenerateai.com",
			PasswordHash:     HashPassword("intern123"),
			Role:             "intern",
			TasksCompleted:   15,
			TasksPending:     3,
			VerificationRate: 88.0,
			CreatedAt:        time.Now(),
		},
	}

	for _, intern := range interns {
		if err := db.Create(&intern).Error; err != nil {
			return err
		}
	}

	return nil
}

func ExtractToken(c *gin.Context) string {
	bearerToken := c.GetHeader("Authorization")
	if strings.HasPrefix(bearerToken, "Bearer ") {
		return strings.TrimPrefix(bearerToken, "Bearer ")
	}
	cookieToken, err := c.Cookie("session_token")
	if err == nil && cookieToken != "" {
		return cookieToken
	}
	return c.Query("token")
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ExtractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication token required"})
			c.Abort()
			return
		}

		session, ok := GetSession(token)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired session token"})
			c.Abort()
			return
		}

		c.Set("userID", session.UserID)
		c.Set("username", session.Username)
		c.Set("userRole", session.Role)
		c.Next()
	}
}

func RequireDevRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("userRole")
		if !exists || role != "dev" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Dev administrator privileges required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func AuthenticateUser(db *gorm.DB, emailOrUsername, password string) (*database.User, string, error) {
	var user database.User
	err := db.Where("email = ? OR username = ?", emailOrUsername, emailOrUsername).First(&user).Error
	if err != nil {
		return nil, "", errors.New("invalid credentials")
	}

	if !VerifyPassword(password, user.PasswordHash) {
		return nil, "", errors.New("invalid credentials")
	}

	now := time.Now()
	user.LastLogin = &now
	db.Model(&user).Update("last_login", now)

	token := GenerateToken(user.ID, user.Username, user.Role)
	return &user, token, nil
}
