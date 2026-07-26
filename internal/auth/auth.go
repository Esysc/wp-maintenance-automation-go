package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type User = db.DBUser
type APIToken = db.DBToken

const (
	RoleAdmin    = "admin"
	RoleUser     = "user"
	RoleReadonly = "readonly"
)

const (
	TokenDuration24h = 24 * time.Hour
	TokenDuration30d = 30 * 24 * time.Hour
	TokenDuration90d = 90 * 24 * time.Hour
	TokenDuration1y  = 365 * 24 * time.Hour
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPasswordChange     = errors.New("password change required")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenRevoked       = errors.New("token revoked")
	ErrWeakPassword       = errors.New("password must be at least 8 characters with uppercase, lowercase, and digit")
)

type AuthManager struct {
	db        *db.Database
	secretKey string
}

func NewAuthManager(dbPath string, secretKey string) (*AuthManager, error) {
	database, err := db.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return &AuthManager{
		db:        database,
		secretKey: secretKey,
	}, nil
}

// InitializeAdmin creates the admin user if it doesn't exist
func (am *AuthManager) InitializeAdmin(username, password string) error {
	// Check if admin already exists
	_, err := am.GetAdminUser()
	if err == nil {
		return fmt.Errorf("admin user already exists")
	}

	admin := User{
		ID:          generateID(),
		Username:    username,
		DisplayName: username,
		Icon:        "user",
		Password:    HashPassword(password),
		Role:        RoleAdmin,
		ForcePass:   true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return am.db.CreateUser(&admin)
}

// LoadUsers loads all users from storage
func (am *AuthManager) LoadUsers() ([]User, error) {
	return am.db.ListUsers()
}

// SaveUsers saves all users to storage
func (am *AuthManager) SaveUsers(users []User) error {
	for _, u := range users {
		u := u
		if err := am.db.UpdateUser(&u); err != nil {
			// If update fails, try create
			_ = am.db.CreateUser(&u)
		}
	}
	return nil
}

// GetUserByID retrieves a user by ID
func (am *AuthManager) GetUserByID(id string) (*User, error) {
	return am.db.GetUserByID(id)
}

// GetUserByUsername retrieves a user by username
func (am *AuthManager) GetUserByUsername(username string) (*User, error) {
	return am.db.GetUserByUsername(username)
}

// Authenticate validates credentials and returns user
func (am *AuthManager) Authenticate(username, password string) (*User, error) {
	user, err := am.GetUserByUsername(username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !VerifyPassword(password, user.Password) {
		return nil, ErrInvalidCredentials
	}

	if user.ForcePass {
		return nil, ErrPasswordChange
	}

	return user, nil
}

// ChangePassword updates user password and clears force password flag
func (am *AuthManager) ChangePassword(userID, newPassword string) error {
	user, err := am.GetUserByID(userID)
	if err != nil {
		return err
	}

	user.Password = HashPassword(newPassword)
	user.ForcePass = false
	user.UpdatedAt = time.Now()

	return am.db.UpdateUser(user)
}

// GetAdminUser returns the admin user
func (am *AuthManager) GetAdminUser() (*User, error) {
	users, err := am.db.ListUsers()
	if err != nil {
		return nil, err
	}

	for i := range users {
		if users[i].Role == RoleAdmin {
			return &users[i], nil
		}
	}

	return nil, ErrUserNotFound
}

// ValidatePassword checks password strength
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

// CreateToken creates a new API token for a user
func (am *AuthManager) CreateToken(userID, name string, duration time.Duration) (*APIToken, error) {
	// Verify user exists
	_, err := am.GetUserByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	token := APIToken{
		ID:        generateID(),
		UserID:    userID,
		Token:     generateToken(),
		Name:      name,
		CreatedAt: time.Now(),
		ExpiresAt: nil,
	}

	if duration > 0 {
		expires := time.Now().Add(duration)
		token.ExpiresAt = &expires
	}

	err = am.db.CreateToken(&token)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &token, nil
}

// LoadTokens loads all API tokens from storage
func (am *AuthManager) LoadTokens() ([]APIToken, error) {
	tokensDB, err := am.db.ListTokens()
	if err != nil {
		return nil, err
	}

	var tokens []APIToken
	for _, t := range tokensDB {
		tokens = append(tokens, *t)
	}
	return tokens, nil
}

// GetTokenByValue retrieves a token by its value
func (am *AuthManager) GetTokenByValue(tokenValue string) (*APIToken, error) {
	tokenDB, err := am.db.GetTokenByValue(tokenValue)
	if err != nil {
		return nil, ErrTokenRevoked
	}

	// Check expiration
	if tokenDB.ExpiresAt != nil && time.Now().After(*tokenDB.ExpiresAt) {
		return nil, ErrTokenExpired
	}

	return tokenDB, nil
}

// RevokeToken marks a token as revoked
func (am *AuthManager) RevokeToken(tokenID string) error {
	err := am.db.RevokeToken(tokenID)
	if err != nil {
		return ErrUserNotFound
	}
	return nil
}

// GenerateID generates a random hex ID
func GenerateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// generateID generates a random hex ID (internal)
func generateID() string {
	return GenerateID()
}

// GenerateSalt generates a random hex salt
func GenerateSalt() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// GenerateToken generates a random API token
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// generateToken generates a random API token (internal)
func generateToken() string {
	return GenerateToken()
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) string {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("failed to hash password: %v", err))
	}
	return string(hashedBytes)
}

// VerifyPassword verifies a password against a hash
func VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// HashPasswordWithSalt is a bcrypt-based password hasher (salt parameter is ignored)
func HashPasswordWithSalt(password, salt string) string {
	return HashPassword(password)
}
