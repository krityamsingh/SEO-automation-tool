package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	IssuedAt  int64
	CreatedAt time.Time
}

// Expired checks if the session has exceeded its maximum lifetime.
func (s *Session) Expired() bool {
	return time.Since(s.CreatedAt) > tokenMaxAge
}

var (
	sessions = make(map[string]Session)
	sessionMu sync.RWMutex
)

// hashIterations is the number of HMAC-SHA256 iterations used by the salted
// scheme. Kept moderate so logins stay fast without external deps.
const hashIterations = 12000

// legacyStaticSalt is the pepper used by the previous (weak, static-salt)
// scheme. It is retained ONLY so existing legacy hashes can still be verified
// and transparently upgraded on next login.
const legacyStaticSalt = "kenerateai_salt_"

// HashPassword produces a salted, iterated hash in the format
// "salt$iterations$hex(hmac)". A random per-user salt is generated each call,
// so identical passwords no longer share the same digest.
func HashPassword(password string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// rand.Read failing is catastrophic; fall back deterministically rather
		// than silently reusing an empty salt.
		fb := sha256.Sum256([]byte(time.Now().String()))
		salt = fb[:]
	}
	return fmt.Sprintf("%s$%d$%s", hex.EncodeToString(salt), hashIterations, hashWithSalt(password, salt, hashIterations))
}

// hashWithSalt applies iterated HMAC-SHA256(key=password, msg=salt||i).
func hashWithSalt(password string, salt []byte, iterations int) string {
	mac := hmac.New(sha256.New, []byte(password))
	mac.Write(salt)
	mac.Write([]byte(strconv.Itoa(iterations)))
	sum := mac.Sum(nil)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(sum)
		sum = mac.Sum(nil)
	}
	return hex.EncodeToString(sum)
}

// VerifyPassword verifies a password against the stored hash. It supports the
// current salted scheme and transparently recognises the legacy static-salt
// scheme (returning true for a correct legacy password so callers can upgrade).
func VerifyPassword(password, stored string) bool {
	// New scheme: "salt$iterations$hexhash"
	if parts := strings.SplitN(stored, "$", 3); len(parts) == 3 {
		salt, err := hex.DecodeString(parts[0])
		if err != nil {
			return false
		}
		iterations, err := strconv.Atoi(parts[1])
		if err != nil || iterations <= 0 {
			return false
		}
		expected, err := hex.DecodeString(parts[2])
		if err != nil {
			return false
		}
		computed, err := hex.DecodeString(hashWithSalt(password, salt, iterations))
		if err != nil {
			return false
		}
		return subtle.ConstantTimeCompare(expected, computed) == 1
	}

	// Legacy scheme: a single unsalted SHA-256 with a static pepper. Verify it
	// with a constant-time compare so we never reveal which scheme matched.
	h := sha256.New()
	h.Write([]byte(legacyStaticSalt + password))
	legacyHex := hex.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(legacyHex), []byte(stored)) == 1
}

// IsLegacyHash reports whether a stored hash uses the weak legacy scheme and
// should be upgraded via HashPassword on the next successful login.
func IsLegacyHash(stored string) bool {
	return !strings.Contains(stored, "$")
}

const tokenSecret = "kenerateai_serverless_jwt_secret_2026"

// tokenMaxAge is the maximum session token lifetime before the user must
// re-authenticate. Applies to both the in-memory store and the stateless
// HMAC-signed fallback used by the Vercel serverless deployment.
const tokenMaxAge = 24 * time.Hour

func GenerateToken(userID uint, username, role string) string {
	ts := time.Now().Unix()
	payload := fmt.Sprintf("%d|%s|%s|%d", userID, username, role, ts)
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
		if sess.Expired() {
			sessionMu.Lock()
			delete(sessions, token)
			sessionMu.Unlock()
			return Session{}, false
		}
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
	if len(fields) < 4 {
		return Session{}, false
	}

	userIDVal, _ := strconv.Atoi(fields[0])
	tsVal, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return Session{}, false
	}

	// Reject tokens older than tokenMaxAge
	if time.Since(time.Unix(tsVal, 0)) > tokenMaxAge {
		return Session{}, false
	}
	if time.Until(time.Unix(tsVal, 0)) > time.Minute {
		return Session{}, false // 1-minute future clock skew tolerance
	}

	sess = Session{
		Token:     token,
		UserID:    uint(userIDVal),
		Username:  fields[1],
		Role:      fields[2],
		CreatedAt: time.Unix(tsVal, 0),
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

	// Upgrade any pre-existing legacy (plaintext / static-salt) seeded hashes so
	// old databases created by the buggy seeder can still log in after upgrade.
	var allUsers []database.User
	db.Find(&allUsers)
	defaults := map[string]string{
		"dev_admin": "admin123",
		"anu":       "intern123",
		"master":    "intern123",
		"hirtik":    "intern123",
		"anuj":      "intern123",
	}
	for i := range allUsers {
		u := &allUsers[i]
		pw, isDefault := defaults[u.Username]
		if isDefault && IsLegacyHash(u.PasswordHash) && VerifyPassword(pw, u.PasswordHash) {
			u.PasswordHash = HashPassword(pw)
			db.Model(u).Update("password_hash", u.PasswordHash)
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

	// Transparently upgrade weak legacy hashes to the salted scheme.
	if IsLegacyHash(user.PasswordHash) {
		user.PasswordHash = HashPassword(password)
		db.Model(&user).Update("password_hash", user.PasswordHash)
	}

	now := time.Now()
	user.LastLogin = &now
	db.Model(&user).Update("last_login", now)

	token := GenerateToken(user.ID, user.Username, user.Role)
	return &user, token, nil
}
