package apiserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/healthcheck"
)

type profileUser struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedOrigins := []string{
			"http://localhost:8080",
			"http://127.0.0.1:8080",
			"https://localhost:8080",
			"https://127.0.0.1:8080",
		}
		origin := r.Header.Get("Origin")

		for _, allowed := range allowedOrigins {
			if origin == allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *APIServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			apiErr(w, http.StatusUnauthorized, "missing authorization")
			return
		}

		tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
		token, err := s.Auth.GetTokenByValue(tokenStr)
		if err != nil {
			apiErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		if token.Revoked {
			apiErr(w, http.StatusUnauthorized, "token revoked")
			return
		}

		user, err := s.Auth.GetUserByID(token.UserID)
		if err != nil {
			apiErr(w, http.StatusUnauthorized, "user not found")
			return
		}

		if user.ForcePass {
			apiErr(w, http.StatusForbidden, "password change required")
			return
		}

		next(w, r)
	}
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func (s *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"server":    "ok",
		"uptime":    time.Now().Unix(),
		"backups":   0,
		"sites":     0,
		"snapshots": 0,
	}

	sites, err := s.Database.ListSites()
	if err == nil {
		status["sites"] = len(sites)
	}

	jsonResp(w, http.StatusOK, status)
}

func (s *APIServer) handleAuthState(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			jsonResp(w, http.StatusOK, map[string]interface{}{
				"setup_required": true,
				"force_pass":     false,
			})
			return
		}
		apiErr(w, http.StatusInternalServerError, "failed to determine auth state")
		return
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"setup_required": admin.ForcePass,
		"force_pass":     admin.ForcePass,
	})
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Password        string `json:"password"`
		PasswordConfirm string `json:"passwordConfirm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Password == "" {
		apiErr(w, http.StatusBadRequest, "password is required")
		return
	}

	// Check if passwords match (for first-time setup)
	admin, err := s.Auth.GetAdminUser()
	isFirstSetup := err != nil

	if isFirstSetup {
		// First-time setup: passwords must match and meet requirements
		if req.Password != req.PasswordConfirm {
			apiErr(w, http.StatusBadRequest, "passwords do not match")
			return
		}

		if err := auth.ValidatePassword(req.Password); err != nil {
			apiErr(w, http.StatusBadRequest, err.Error())
			return
		}

		// Create admin user
		adminID := auth.GenerateID()
		admin = &auth.User{
			ID:          adminID,
			Username:    "internal",
			DisplayName: "Administrator",
			Icon:        "user",
			Password:    auth.HashPassword(req.Password),
			Role:        auth.RoleAdmin,
			ForcePass:   false,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		// Save user to DB
		dbUser := &db.DBUser{
			ID:          adminID,
			Username:    "internal",
			DisplayName: "Administrator",
			Icon:        "user",
			Password:    admin.Password,
			Role:        auth.RoleAdmin,
			ForcePass:   false,
		}
		if err := s.Database.CreateUser(dbUser); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to create admin user")
			return
		}
	} else {
		// Admin user exists - verify password
		if admin.ForcePass {
			if req.Password != req.PasswordConfirm {
				apiErr(w, http.StatusBadRequest, "passwords do not match")
				return
			}
			if err := auth.ValidatePassword(req.Password); err != nil {
				apiErr(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := s.Auth.ChangePassword(admin.ID, req.Password); err != nil {
				apiErr(w, http.StatusInternalServerError, "failed to set password")
				return
			}
			admin.ForcePass = false
		} else {
			if !auth.VerifyPassword(req.Password, admin.Password) {
				apiErr(w, http.StatusUnauthorized, "invalid password")
				return
			}
		}
	}

	token, err := s.Auth.CreateToken(admin.ID, "web-session", 24*time.Hour)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"token":        token.Token,
		"user_id":      admin.ID,
		"username":     admin.Username,
		"display_name": profilePayload(admin).DisplayName,
		"icon":         profilePayload(admin).Icon,
		"role":         admin.Role,
		"force_pass":   false,
		"first_login":  isFirstSetup,
	})
}

func (s *APIServer) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		UserID      string `json:"user_id"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.Auth.ChangePassword(req.UserID, req.NewPassword); err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to change password")
		return
	}

	jsonResp(w, http.StatusOK, map[string]string{"message": "password changed"})
}

func (s *APIServer) handleUsers(w http.ResponseWriter, r *http.Request) {
	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		apiErr(w, http.StatusNotFound, "internal user not found")
		return
	}

	switch r.Method {
	case "GET":
		jsonResp(w, http.StatusOK, profilePayload(admin))

	case "PUT":
		var req struct {
			DisplayName string `json:"display_name"`
			Icon        string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		updated, err := s.updateProfile(admin, req.DisplayName, req.Icon)
		if err != nil {
			apiErr(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResp(w, http.StatusOK, profilePayload(updated))

	case "POST":
		apiErr(w, http.StatusForbidden, "single-user mode does not support creating users")

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *APIServer) handleUserByID(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	admin, err := s.Auth.GetAdminUser()
	if err != nil {
		apiErr(w, http.StatusNotFound, "internal user not found")
		return
	}

	if userID != admin.ID {
		apiErr(w, http.StatusNotFound, "user not found")
		return
	}

	switch r.Method {
	case "GET":
		jsonResp(w, http.StatusOK, profilePayload(admin))

	case "PUT":
		var req struct {
			DisplayName string `json:"display_name"`
			Icon        string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		updated, err := s.updateProfile(admin, req.DisplayName, req.Icon)
		if err != nil {
			apiErr(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonResp(w, http.StatusOK, profilePayload(updated))

	case "DELETE":
		apiErr(w, http.StatusForbidden, "internal user cannot be deleted")

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *APIServer) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		tokens, err := s.Auth.LoadTokens()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to list tokens")
			return
		}

		currentToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		visibleTokens := make([]map[string]interface{}, 0, len(tokens))
		for _, token := range tokens {
			isCurrentSession := currentToken != "" && token.Token == currentToken
			isInternalSession := strings.TrimSpace(token.Name) == "web-session"

			// Hide internal session tokens unless they are the active web session.
			if isInternalSession && !isCurrentSession {
				continue
			}

			visibleTokens = append(visibleTokens, map[string]interface{}{
				"id":              token.ID,
				"user_id":         token.UserID,
				"name":            token.Name,
				"created_at":      token.CreatedAt,
				"expires_at":      token.ExpiresAt,
				"last_used":       token.LastUsed,
				"revoked":         token.Revoked,
				"current_session": isCurrentSession,
			})
		}

		jsonResp(w, http.StatusOK, visibleTokens)

	case "POST":
		var req struct {
			Name     string `json:"name"`
			Duration int    `json:"duration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid request body")
			return
		}

		admin, err := s.Auth.GetAdminUser()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to get internal user")
			return
		}

		duration := time.Duration(req.Duration) * time.Hour
		token, err := s.Auth.CreateToken(admin.ID, req.Name, duration)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to create token: "+err.Error())
			return
		}
		jsonResp(w, http.StatusCreated, map[string]interface{}{
			"id":    token.ID,
			"token": token.Token,
			"name":  token.Name,
		})

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func profilePayload(user *auth.User) profileUser {
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = user.Username
	}

	icon := normalizeProfileIcon(user.Icon)

	return profileUser{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: displayName,
		Icon:        icon,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

func (s *APIServer) updateProfile(user *auth.User, displayName, icon string) (*auth.User, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("display name is required")
	}

	user.DisplayName = displayName
	user.Icon = normalizeProfileIcon(icon)
	user.UpdatedAt = time.Now()

	if err := s.Database.UpdateUser(user); err != nil {
		return nil, fmt.Errorf("failed to update profile")
	}

	return user, nil
}

func normalizeProfileIcon(icon string) string {
	switch strings.TrimSpace(icon) {
	case "shield", "bolt", "globe", "gear", "user":
		return strings.TrimSpace(icon)
	default:
		return "user"
	}
}

func (s *APIServer) handleTokenByID(w http.ResponseWriter, r *http.Request) {
	tokenID := strings.TrimPrefix(r.URL.Path, "/api/v1/tokens/")

	if r.Method == "DELETE" {
		if err := s.Auth.RevokeToken(tokenID); err != nil {
			apiErr(w, http.StatusNotFound, "token not found")
			return
		}
		jsonResp(w, http.StatusOK, map[string]string{"message": "token revoked"})
		return
	}

	apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *APIServer) handleBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Get site_id from query param or use all backups
	siteID := r.URL.Query().Get("site_id")
	var backups []*db.Backup
	var err error

	if siteID != "" {
		backups, err = s.Database.GetBackupsBySite(siteID)
	} else {
		// Return empty list if no site_id provided
		backups = []*db.Backup{}
	}

	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to list backups")
		return
	}

	jsonResp(w, http.StatusOK, backups)
}

func (s *APIServer) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SiteID string `json:"site_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SiteID == "" {
		apiErr(w, http.StatusBadRequest, "site_id is required")
		return
	}

	// Create backup record in database
	backup := &db.Backup{
		ID:        auth.GenerateID(),
		SiteID:    req.SiteID,
		Timestamp: time.Now().Format("20060102_150405"),
		Host:      "unknown",
		WPRoot:    "/var/www/html",
		FileCount: 0,
	}

	if err := s.Database.CreateBackup(backup); err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to create backup record")
		return
	}

	jsonResp(w, http.StatusOK, map[string]string{"message": "backup initiated", "status": "running", "backup_id": backup.ID})
}

func (s *APIServer) handleBackupByID(w http.ResponseWriter, r *http.Request) {
	_ = strings.TrimPrefix(r.URL.Path, "/api/v1/backups/")

	switch r.Method {
	case "GET":
		// For now, return not implemented for individual backup
		apiErr(w, http.StatusNotImplemented, "individual backup retrieval not implemented")
		return

	case "DELETE":
		// For now, delete is not implemented
		apiErr(w, http.StatusNotImplemented, "backup deletion not implemented")
		return

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *APIServer) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SnapshotID string `json:"snapshot_id"`
		ApplyDB    bool   `json:"apply_db"`
		ApplyFiles bool   `json:"apply_files"`
		Confirm    bool   `json:"confirm"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SnapshotID == "" {
		apiErr(w, http.StatusBadRequest, "snapshot_id is required")
		return
	}

	if !req.Confirm {
		jsonResp(w, http.StatusOK, map[string]interface{}{
			"message":     "restore initiated",
			"snapshot_id": req.SnapshotID,
			"apply_db":    req.ApplyDB,
			"apply_files": req.ApplyFiles,
			"status":      "running",
			"warning":     "Set confirm=true to actually apply the restore",
		})
		return
	}

	go func() {
		// Restore not implemented yet - site-based restore will be added later
		log.Printf("Restore not implemented for snapshot: %s", req.SnapshotID)
	}()

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"message":     "restore initiated",
		"snapshot_id": req.SnapshotID,
		"apply_db":    req.ApplyDB,
		"apply_files": req.ApplyFiles,
		"status":      "running",
	})
}

func (s *APIServer) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if s.Restic == nil {
		apiErr(w, http.StatusServiceUnavailable, "restic not configured")
		return
	}

	snapshots, err := s.Restic.Snapshots()
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to list snapshots")
		return
	}

	jsonResp(w, http.StatusOK, snapshots)
}

func (s *APIServer) handleSnapshotByID(w http.ResponseWriter, r *http.Request) {
	snapshotID := strings.TrimPrefix(r.URL.Path, "/api/v1/snapshots/")

	if s.Restic == nil {
		apiErr(w, http.StatusServiceUnavailable, "restic not configured")
		return
	}

	switch r.Method {
	case "GET":
		snapshots, err := s.Restic.Snapshots()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to get snapshots")
			return
		}
		for _, snap := range snapshots {
			if snap.ID == snapshotID || snap.ShortID == snapshotID {
				jsonResp(w, http.StatusOK, snap)
				return
			}
		}
		apiErr(w, http.StatusNotFound, "snapshot not found")

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *APIServer) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	go func() {
		// Upgrade not implemented yet - site-based upgrade will be added later
		log.Printf("Upgrade not implemented")
	}()

	jsonResp(w, http.StatusOK, map[string]string{"message": "upgrade initiated", "status": "running"})
}

func (s *APIServer) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		URL          string `json:"url"`
		ExpectedCode int    `json:"expected_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	opts := healthcheck.NewDefaultOptions(req.URL)
	if req.ExpectedCode > 0 {
		opts.ExpectedCode = req.ExpectedCode
	}

	result, err := s.Checker.Check(opts)
	if err != nil {
		apiErr(w, http.StatusOK, fmt.Sprintf("healthcheck failed: %v", err))
		return
	}

	jsonResp(w, http.StatusOK, result)
}

func (s *APIServer) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		sites, err := s.Database.ListSites()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to list sites")
			return
		}
		jsonResp(w, http.StatusOK, sites)

	case "POST":
		var site db.Site
		if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		site.ID = auth.GenerateID()
		if err := s.Database.CreateSite(&site); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to create site")
			return
		}
		jsonResp(w, http.StatusCreated, site)

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *APIServer) handleSiteByID(w http.ResponseWriter, r *http.Request) {
	siteID := strings.TrimPrefix(r.URL.Path, "/api/v1/sites/")

	switch r.Method {
	case "GET":
		site, err := s.Database.GetSite(siteID)
		if err != nil {
			apiErr(w, http.StatusNotFound, "site not found")
			return
		}
		jsonResp(w, http.StatusOK, site)

	case "PUT":
		var site db.Site
		if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		site.ID = siteID
		if err := s.Database.UpdateSite(&site); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to update site")
			return
		}
		jsonResp(w, http.StatusOK, map[string]string{"message": "site updated"})

	case "DELETE":
		if err := s.Database.DeleteSite(siteID); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to delete site")
			return
		}
		jsonResp(w, http.StatusOK, map[string]string{"message": "site deleted"})

	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func jsonResp(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": status >= 200 && status < 300,
		"data":    data,
	})
}

func apiErr(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

func (s *APIServer) handleDetectConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SSHHost  string `json:"ssh_host"`
		SSHPort  int    `json:"ssh_port"`
		SSHUser  string `json:"ssh_user"`
		WPRoot   string `json:"wp_root"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sshClient, err := ssh.NewClient(req.SSHHost, req.SSHPort, req.SSHUser, "")
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to connect via SSH")
		return
	}

	dbName, dbUser, dbPass, dbHost, err := sshClient.ParseDBConfig(req.WPRoot)
	if err != nil {
		apiErr(w, http.StatusOK, "could not parse database config")
		return
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"db_host":     dbHost,
			"db_user":     dbUser,
			"db_password": dbPass,
			"db_name":     dbName,
		},
	})
}
