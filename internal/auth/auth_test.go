package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestAuth(t *testing.T) *AuthManager {
	t.Helper()
	dir, err := os.MkdirTemp("", "auth-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	am, err := NewAuthManager(dbPath, "test-secret-key")
	if err != nil {
		t.Fatalf("failed to create auth manager: %v", err)
	}

	t.Cleanup(func() {
		am.db.Close()
		os.RemoveAll(dir)
	})
	return am
}

func TestNewAuthManager(t *testing.T) {
	am := setupTestAuth(t)
	if am == nil {
		t.Fatal("expected non-nil auth manager")
	}
}

func TestInitializeAdmin(t *testing.T) {
	am := setupTestAuth(t)

	err := am.InitializeAdmin("admin", "adminpass123")
	if err != nil {
		t.Fatalf("failed to initialize admin: %v", err)
	}

	users, err := am.LoadUsers()
	if err != nil {
		t.Fatalf("failed to load users: %v", err)
	}

	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}

	if users[0].Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", users[0].Username)
	}

	if users[0].Role != RoleAdmin {
		t.Errorf("expected role 'admin', got '%s'", users[0].Role)
	}

	if !users[0].ForcePass {
		t.Error("expected force_pass to be true for new admin")
	}
}

func TestInitializeAdminDuplicate(t *testing.T) {
	am := setupTestAuth(t)

	am.InitializeAdmin("admin", "pass1")
	err := am.InitializeAdmin("admin2", "pass2")

	if err == nil {
		t.Fatal("expected error for duplicate admin, got nil")
	}
}

func TestAuthenticate(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	_, err := am.Authenticate("admin", "password123")
	if err == nil {
		t.Fatal("expected password change required error")
	}
	if err != ErrPasswordChange {
		t.Fatalf("expected ErrPasswordChange, got %v", err)
	}
}

func TestAuthenticateAfterPasswordChange(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID

	err := am.ChangePassword(adminID, "newpassword456")
	if err != nil {
		t.Fatalf("failed to change password: %v", err)
	}

	user, err := am.Authenticate("admin", "newpassword456")
	if err != nil {
		t.Fatalf("authentication failed after password change: %v", err)
	}

	if user.ForcePass {
		t.Error("expected force_pass to be false after password change")
	}
}

func TestAuthenticateInvalidCredentials(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID
	am.ChangePassword(adminID, "newpassword456")

	_, err := am.Authenticate("admin", "wrongpassword")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestCreateToken(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID
	am.ChangePassword(adminID, "newpass")

	token, err := am.CreateToken(adminID, "test-token", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	if token.Name != "test-token" {
		t.Errorf("expected name 'test-token', got '%s'", token.Name)
	}

	if token.Revoked {
		t.Error("expected new token to not be revoked")
	}

	if token.Token == "" {
		t.Error("expected non-empty token value")
	}
}

func TestGetTokenByValue(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID
	am.ChangePassword(adminID, "newpass")

	created, _ := am.CreateToken(adminID, "test-token", 24*time.Hour)

	found, err := am.GetTokenByValue(created.Token)
	if err != nil {
		t.Fatalf("failed to find token by value: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("expected token ID %s, got %s", created.ID, found.ID)
	}
}

func TestRevokeToken(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID
	am.ChangePassword(adminID, "newpass")

	created, _ := am.CreateToken(adminID, "test-token", 24*time.Hour)

	err := am.RevokeToken(created.ID)
	if err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	_, err = am.GetTokenByValue(created.Token)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
}

func TestLoadUsers(t *testing.T) {
	am := setupTestAuth(t)

	users, err := am.LoadUsers()
	if err != nil {
		t.Fatalf("failed to load users: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestGetUserByID(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID

	user, err := am.GetUserByID(adminID)
	if err != nil {
		t.Fatalf("failed to get user by ID: %v", err)
	}

	if user.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", user.Username)
	}
}

func TestGetUserByUsername(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	user, err := am.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("failed to get user by username: %v", err)
	}

	if user.Role != RoleAdmin {
		t.Errorf("expected role 'admin', got '%s'", user.Role)
	}
}

func TestChangePassword(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID

	err := am.ChangePassword(adminID, "newpassword")
	if err != nil {
		t.Fatalf("failed to change password: %v", err)
	}

	u, _ := am.GetUserByID(adminID)
	if u.ForcePass {
		t.Error("expected force_pass to be false after password change")
	}
}

func TestGetUserNotFound(t *testing.T) {
	am := setupTestAuth(t)

	_, err := am.GetUserByID("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
}

func TestTokenExpiration(t *testing.T) {
	am := setupTestAuth(t)
	am.InitializeAdmin("admin", "password123")

	users, _ := am.LoadUsers()
	adminID := users[0].ID
	am.ChangePassword(adminID, "newpass")

	// Create token with 0 duration (no expiry)
	token, err := am.CreateToken(adminID, "no-expiry", 0)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	found, err := am.GetTokenByValue(token.Token)
	if err != nil {
		t.Fatalf("failed to find token: %v", err)
	}
	if found.ID != token.ID {
		t.Errorf("token ID mismatch")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()

	if id1 == id2 {
		t.Error("expected unique IDs")
	}

	if len(id1) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars", len(id1))
	}
}

func TestHashPassword(t *testing.T) {
	hash1 := HashPassword("password123")
	hash2 := HashPassword("password123")

	if hash1 == hash2 {
		t.Error("expected different hashes due to random salts")
	}
}

func TestHashPasswordWithSalt(t *testing.T) {
	salt := GenerateSalt()
	hash1 := HashPasswordWithSalt("password123", salt)
	hash2 := HashPasswordWithSalt("password123", salt)

	// bcrypt generates a new salt internally, so hashes will differ
	if hash1 == hash2 {
		t.Error("expected different hashes due to bcrypt random salt")
	}

	if !VerifyPassword("password123", hash1) {
		t.Error("expected hash1 to verify against password")
	}

	if !VerifyPassword("password123", hash2) {
		t.Error("expected hash2 to verify against password")
	}
}
