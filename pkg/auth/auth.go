package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/database"
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

const tokenSecret = "kenerateai_serverless_jwt_secret_2026"

func GenerateToken(userID uint, username, role string) string {
	payload := fmt.Sprintf("%d|%s|%s|%d", userID, username, role, time.Now().Unix())
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, []byte(tokenSecret))
	mac.Write([]byte(encoded))
	signature := hex.EncodeToString(mac.Sum(nil))

	token := encoded + "." + signature

	sessionMu.Lock()
	sessions[token] = Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		Role:      role,
		CreatedAt: time.Now(),
	}
	sessionMu.Unlock()

	return token
}

func GetSession(token string) (Session, bool) {
	sessionMu.RLock()
	sess, ok := sessions[token]
	sessionMu.RUnlock()
	if ok {
		return sess, true
	}

	// Stateless verification fallback for Vercel multi-instance lambda execution
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Session{}, false
	}
	encodedPayload, sig := parts[0], parts[1]

	mac := hmac.New(sha256.New, []byte(tokenSecret))
	mac.Write([]byte(encodedPayload))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return Session{}, false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return Session{}, false
	}

	fields := strings.Split(string(payloadBytes), "|")
	if len(fields) < 3 {
		return Session{}, false
	}

	userIDVal, _ := strconv.Atoi(fields[0])
	sess = Session{
		Token:     token,
		UserID:    uint(userIDVal),
		Username:  fields[1],
		Role:      fields[2],
		CreatedAt: time.Now(),
	}

	sessionMu.Lock()
	sessions[token] = sess
	sessionMu.Unlock()

	return sess, true
}

func RevokeToken(token string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessions, token)
}

func SeedDefaultUsers(db *gorm.DB) error {
	// Create default Dev user if missing
	var devUser database.User
	if db.Where("username = ?", "dev_admin").First(&devUser).Error != nil {
		devUser = database.User{
			Username:         "dev_admin",
			Email:            "dev@kenerateai.com",
			PasswordHash:     HashPassword("admin123"),
			Role:             "dev",
			TasksCompleted:   0,
			TasksPending:     0,
			VerificationRate: 100.0,
			CreatedAt:        time.Now(),
		}
		db.Create(&devUser)
	}

	// Create requested Intern users: anu, master, hirtik, anuj
	internNames := []string{"anu", "master", "hirtik", "anuj"}
	for i, name := range internNames {
		var existing database.User
		if db.Where("username = ?", name).First(&existing).Error != nil {
			intern := database.User{
				Username:         name,
				Email:            fmt.Sprintf("%s@kenerateai.com", name),
				PasswordHash:     HashPassword("intern123"),
				Role:             "intern",
				TasksCompleted:   (i + 1) * 3,
				TasksPending:     1,
				VerificationRate: 95.0,
				CreatedAt:        time.Now(),
			}
			db.Create(&intern)
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
