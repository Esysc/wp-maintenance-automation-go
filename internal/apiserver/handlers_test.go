package apiserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/healthcheck"
)

func setupTestAPIServer(t *testing.T) *APIServer {
	t.Helper()

	dir, err := os.MkdirTemp("", "apiserver-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(dir, "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	secretKey := "test-secret-key"
	authMgr := auth.NewAuthManager(database, secretKey)

	checker := healthcheck.NewChecker()

	s := &APIServer{
		Auth:     authMgr,
		Database: database,
		Checker:  checker,
	}

	t.Cleanup(func() {
		database.Close()
		os.RemoveAll(dir)
	})

	return s
}

func TestHandleLoginFirstTime(t *testing.T) {
	s := setupTestAPIServer(t)

	reqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
		return
	}

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result["success"] != true {
		t.Fatalf("Expected success true, got %v", result["success"])
	}

	data := result["data"].(map[string]interface{})
	if isFirstLogin, ok := data["first_login"].(bool); !ok || !isFirstLogin {
		t.Fatalf("Expected first_login to be true, got %v", data["first_login"])
	}

	if token, ok := data["token"].(string); !ok || token == "" {
		t.Fatalf("Expected token to be present, got %v", data["token"])
	}
}

func TestHandleLoginPasswordsDoNotMatch(t *testing.T) {
	s := setupTestAPIServer(t)

	reqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "DifferentPass123",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rr.Code)
	}
}

func TestHandleLoginWeakPassword(t *testing.T) {
	s := setupTestAPIServer(t)

	reqBody := map[string]string{
		"password":        "weak",
		"passwordConfirm": "weak",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", rr.Code)
	}
}

func TestHandleAuthStateFirstSetup(t *testing.T) {
	s := setupTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v1/auth/state", nil)
	rr := httptest.NewRecorder()
	s.handleAuthState(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result["success"] != true {
		t.Fatalf("Expected success true, got %v", result["success"])
	}

	data := result["data"].(map[string]interface{})
	if setupRequired, ok := data["setup_required"].(bool); !ok || !setupRequired {
		t.Fatalf("Expected setup_required true, got %v", data["setup_required"])
	}
}

func TestHandleAuthStateConfigured(t *testing.T) {
	s := setupTestAPIServer(t)

	// First login creates the internal admin user and clears force_pass.
	reqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	req := httptest.NewRequest("GET", "/api/v1/auth/state", nil)
	rr := httptest.NewRecorder()
	s.handleAuthState(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result["success"] != true {
		t.Fatalf("Expected success true, got %v", result["success"])
	}

	data := result["data"].(map[string]interface{})
	if setupRequired, ok := data["setup_required"].(bool); !ok || setupRequired {
		t.Fatalf("Expected setup_required false, got %v", data["setup_required"])
	}
}

func TestHandleLoginSecondTime(t *testing.T) {
	s := setupTestAPIServer(t)

	// First login
	reqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)
	firstLogin := result["data"].(map[string]interface{})["first_login"].(bool)
	if !firstLogin {
		t.Fatal("Expected first_login to be true")
	}

	// Second login
	req2 := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	s.handleLogin(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("Expected status 200 on second login, got %d. Body: %s", rr2.Code, rr2.Body.String())
	}

	var result2 map[string]interface{}
	json.Unmarshal(rr2.Body.Bytes(), &result2)
	secondLogin := result2["data"].(map[string]interface{})["first_login"].(bool)
	if secondLogin {
		t.Fatal("Expected first_login to be false on second login")
	}
}

func TestHandleChangePasswordRequiresCurrentPassword(t *testing.T) {
	s := setupTestAPIServer(t)

	loginReqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	loginBodyBytes, _ := json.Marshal(loginReqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	var loginResult map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResult)
	data := loginResult["data"].(map[string]interface{})
	adminToken := data["token"].(string)
	adminUserID := data["user_id"].(string)

	reqBody := map[string]string{
		"user_id":      adminUserID,
		"new_password": "Newpassword123",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	rr := httptest.NewRecorder()
	s.authMiddleware(s.handleChangePassword)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleChangePasswordInvalidCurrentPassword(t *testing.T) {
	s := setupTestAPIServer(t)

	loginReqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	loginBodyBytes, _ := json.Marshal(loginReqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	var loginResult map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResult)
	data := loginResult["data"].(map[string]interface{})
	adminToken := data["token"].(string)
	adminUserID := data["user_id"].(string)

	reqBody := map[string]string{
		"user_id":          adminUserID,
		"current_password": "WrongPass123",
		"new_password":     "Newpassword123",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	rr := httptest.NewRecorder()
	s.authMiddleware(s.handleChangePassword)(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("Expected status 401, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleTokensCreateDefaultUser(t *testing.T) {
	s := setupTestAPIServer(t)

	// First, create a login to get admin user
	loginReqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	loginBodyBytes, _ := json.Marshal(loginReqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	var loginResult map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResult)
	tokenData := loginResult["data"].(map[string]interface{})
	adminToken := tokenData["token"].(string)

	// Now create a token without user_id
	tokenReqBody := map[string]interface{}{
		"name":     "Test Token",
		"duration": 24,
	}
	tokenBodyBytes, _ := json.Marshal(tokenReqBody)

	tokenReq := httptest.NewRequest("POST", "/api/v1/tokens", bytes.NewBuffer(tokenBodyBytes))
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenReq.Header.Set("Authorization", "Bearer "+adminToken)

	tokenRR := httptest.NewRecorder()
	s.handleTokens(tokenRR, tokenReq)

	if tokenRR.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d. Body: %s", tokenRR.Code, tokenRR.Body.String())
	}

	var tokenResult map[string]interface{}
	json.Unmarshal(tokenRR.Body.Bytes(), &tokenResult)

	if tokenResult["success"] != true {
		t.Fatalf("Expected success true, got %v", tokenResult["success"])
	}

	if tokenID, ok := tokenResult["data"].(map[string]interface{})["id"].(string); !ok || tokenID == "" {
		t.Fatalf("Expected token ID to be present, got %v", tokenResult["data"])
	}
}

func TestHandleSitesCreateAndGet(t *testing.T) {
	s := setupTestAPIServer(t)

	// Create login first
	loginReqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	loginBodyBytes, _ := json.Marshal(loginReqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	var loginResult map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResult)
	tokenData := loginResult["data"].(map[string]interface{})
	adminToken := tokenData["token"].(string)

	// Create site
	siteReqBody := map[string]interface{}{
		"name":            "Test Site",
		"wp_ssh_host":     "example.com",
		"wp_ssh_port":     22,
		"wp_ssh_user":     "ubuntu",
		"wp_root":         "/var/www/html",
		"db_host":         "localhost",
		"db_user":         "wpuser",
		"db_password":     "wppass",
		"db_name":         "wordpress",
		"backup_dir":      "./backups",
		"retention_flags": "--keep-daily 7",
		"healthcheck_url": "http://example.com",
		"staging_enabled": false,
	}
	siteBodyBytes, _ := json.Marshal(siteReqBody)

	siteReq := httptest.NewRequest("POST", "/api/v1/sites", bytes.NewBuffer(siteBodyBytes))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+adminToken)

	siteRR := httptest.NewRecorder()
	s.handleSites(siteRR, siteReq)

	if siteRR.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d. Body: %s", siteRR.Code, siteRR.Body.String())
	}

	var siteResult map[string]interface{}
	json.Unmarshal(siteRR.Body.Bytes(), &siteResult)

	if siteResult["success"] != true {
		t.Fatalf("Expected success true, got %v", siteResult["success"])
	}

	siteID := siteResult["data"].(map[string]interface{})["id"].(string)

	// Get site
	getSiteReq := httptest.NewRequest("GET", "/api/v1/sites/"+siteID, nil)
	getSiteReq.Header.Set("Authorization", "Bearer "+adminToken)

	getSiteRR := httptest.NewRecorder()
	s.handleSiteByID(getSiteRR, getSiteReq)

	if getSiteRR.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", getSiteRR.Code)
	}

	var getSiteResult map[string]interface{}
	json.Unmarshal(getSiteRR.Body.Bytes(), &getSiteResult)

	if getSiteResult["success"] != true {
		t.Fatalf("Expected success true, got %v", getSiteResult["success"])
	}

	if siteName := getSiteResult["data"].(map[string]interface{})["name"].(string); siteName != "Test Site" {
		t.Fatalf("Expected name 'Test Site', got '%s'", siteName)
	}
}

func TestHandleSitesList(t *testing.T) {
	s := setupTestAPIServer(t)

	// Create login first
	loginReqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	loginBodyBytes, _ := json.Marshal(loginReqBody)
	loginReq := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginBodyBytes))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	s.handleLogin(loginRR, loginReq)

	var loginResult map[string]interface{}
	json.Unmarshal(loginRR.Body.Bytes(), &loginResult)
	adminToken := loginResult["data"].(map[string]interface{})["token"].(string)

	// Create site
	siteReqBody := map[string]interface{}{
		"name":            "Test Site 2",
		"wp_ssh_host":     "example2.com",
		"wp_ssh_port":     22,
		"wp_ssh_user":     "ubuntu",
		"wp_root":         "/var/www/html",
		"backup_dir":      "./backups",
		"retention_flags": "--keep-daily 7",
	}
	siteBodyBytes, _ := json.Marshal(siteReqBody)

	siteReq := httptest.NewRequest("POST", "/api/v1/sites", bytes.NewBuffer(siteBodyBytes))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer "+adminToken)

	siteRR := httptest.NewRecorder()
	s.handleSites(siteRR, siteReq)

	// List sites
	listSitesReq := httptest.NewRequest("GET", "/api/v1/sites", nil)
	listSitesReq.Header.Set("Authorization", "Bearer "+adminToken)

	listSitesRR := httptest.NewRecorder()
	s.handleSites(listSitesRR, listSitesReq)

	if listSitesRR.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", listSitesRR.Code)
	}

	var listSitesResult map[string]interface{}
	json.Unmarshal(listSitesRR.Body.Bytes(), &listSitesResult)

	if listSitesResult["success"] != true {
		t.Fatalf("Expected success true, got %v", listSitesResult["success"])
	}

	sites := listSitesResult["data"].([]interface{})
	if len(sites) != 1 {
		t.Fatalf("Expected 1 site, got %d", len(sites))
	}
}

func TestHandleUsersReturnsInternalProfile(t *testing.T) {
	s := setupTestAPIServer(t)
	adminToken := loginTestAdmin(t, s)

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	s.handleUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)
	data := result["data"].(map[string]interface{})

	if data["username"] != "internal" {
		t.Fatalf("expected username internal, got %v", data["username"])
	}
	if data["display_name"] != "Administrator" {
		t.Fatalf("expected default display name Administrator, got %v", data["display_name"])
	}
	if data["icon"] != "user" {
		t.Fatalf("expected default icon user, got %v", data["icon"])
	}
}

func TestHandleUsersUpdateProfile(t *testing.T) {
	s := setupTestAPIServer(t)
	adminToken := loginTestAdmin(t, s)

	bodyBytes, _ := json.Marshal(map[string]string{
		"display_name": "Andrea",
		"icon":         "shield",
	})

	req := httptest.NewRequest("PUT", "/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	s.handleUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		t.Fatalf("failed to reload admin: %v", err)
	}

	if admin.DisplayName != "Andrea" {
		t.Fatalf("expected updated display name Andrea, got %q", admin.DisplayName)
	}
	if admin.Icon != "shield" {
		t.Fatalf("expected updated icon shield, got %q", admin.Icon)
	}
}

func TestHandleUsersCreateRejectedInSingleUserMode(t *testing.T) {
	s := setupTestAPIServer(t)
	adminToken := loginTestAdmin(t, s)

	bodyBytes, _ := json.Marshal(map[string]string{
		"username": "second",
		"password": "SecondPass123",
	})

	req := httptest.NewRequest("POST", "/api/v1/users", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	s.handleUsers(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("Expected status 403, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleInternalUserDeleteRejected(t *testing.T) {
	s := setupTestAPIServer(t)
	adminToken := loginTestAdmin(t, s)
	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		t.Fatalf("failed to get admin: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/users/"+admin.ID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	s.handleUserByID(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("Expected status 403, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleTokensAlwaysLinkToInternalUser(t *testing.T) {
	s := setupTestAPIServer(t)
	adminToken := loginTestAdmin(t, s)
	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		t.Fatalf("failed to get admin: %v", err)
	}

	otherUser := &db.DBUser{
		ID:          "user-2",
		Username:    "other",
		DisplayName: "Other",
		Icon:        "gear",
		Password:    auth.HashPassword("OtherPass123"),
		Role:        auth.RoleUser,
		ForcePass:   false,
	}
	if err := s.Database.CreateUser(otherUser); err != nil {
		t.Fatalf("failed to create extra user: %v", err)
	}

	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"user_id":  "user-2",
		"name":     "Scoped Token",
		"duration": 24,
	})

	req := httptest.NewRequest("POST", "/api/v1/tokens", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	s.handleTokens(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	tokens, err := s.Database.ListTokens()
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	found := false
	for _, token := range tokens {
		if token.Name == "Scoped Token" {
			found = true
			if token.UserID != admin.ID {
				t.Fatalf("expected token user id %q, got %q", admin.ID, token.UserID)
			}
		}
	}

	if !found {
		t.Fatalf("expected to find created token")
	}
}

func loginTestAdmin(t *testing.T, s *APIServer) string {
	t.Helper()
	reqBody := map[string]string{
		"password":        "AdminPass123",
		"passwordConfirm": "AdminPass123",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &result)
	return result["data"].(map[string]interface{})["token"].(string)
}
