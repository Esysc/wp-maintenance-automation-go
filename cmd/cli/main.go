package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
)

type CLIClient struct {
	baseURL  string
	apiToken string
	client   *http.Client
}

func NewCLIClient(baseURL, apiToken string) *CLIClient {
	return &CLIClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiToken: apiToken,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CLIClient) do(method, path string, body interface{}) (map[string]interface{}, error) {
	url := c.baseURL + "/api/v1" + path

	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
	}

	req, err := http.NewRequest(method, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	baseURL := os.Getenv("API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}

	apiToken := os.Getenv("WP_MAINTENANCE_TOKEN")

	client := NewCLIClient(baseURL, apiToken)
	command := os.Args[1]

	switch command {
	case "backup":
		cmdBackup(client, os.Args[2:])
	case "restore":
		cmdRestore(client, os.Args[2:])
	case "upgrade":
		cmdUpgrade(client, os.Args[2:])
	case "backups":
		cmdListBackups(client)
	case "snapshots":
		cmdListSnapshots(client)
	case "healthcheck":
		cmdHealthcheck(client, os.Args[2:])
	case "status":
		cmdStatus(client)
	case "login":
		cmdLogin(client, os.Args[2:])
	case "user":
		cmdUser(client, os.Args[2:])
	case "token":
		cmdToken(client, os.Args[2:])
	case "config":
		cmdConfig(client, os.Args[2:])
	case "reset-password":
		cmdResetPassword(client, os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`WP Maintenance Automation CLI v1.0.0

Usage:
  wp-maintenance <command> [options]

Commands:
  backup                  Create a new backup
  restore <snapshot_id>   Restore from a snapshot
  upgrade                 Run full WordPress upgrade
  backups                 List all backups
  snapshots               List restic snapshots
  healthcheck <url>       Run healthcheck against a URL
  status                  Show system status
  login                   Login to get API token
  user                    Manage users
    user list             List users
    user create <user> <pass> <role>   Create user
    user delete <id>      Delete user
  token                   Manage API tokens
    token list            List tokens
    token create <user_id> <name> [hours]   Create token
    token revoke <id>     Revoke token
  config                  View/update configuration
	reset-password          Reset internal admin password (local DB recovery)
  help                    Show this help message

Environment:
  API_URL              API server URL (default: http://localhost:8081)
  WP_MAINTENANCE_TOKEN  API token for authentication
	DATA_DIR              Data directory (default: ./data)
	SECRET_KEY            Signing key (default: default-secret-key)
`)
}

func cmdBackup(client *CLIClient, args []string) {
	fmt.Println("Initiating backup...")
	result, err := client.do("POST", "/backup", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdRestore(client *CLIClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: wp-maintenance restore <snapshot_id>")
		os.Exit(1)
	}

	snapshotID := args[0]
	applyDB := false
	applyFiles := false

	for i, arg := range args {
		if arg == "--apply-db" || arg == "-d" {
			applyDB = true
		}
		if arg == "--apply-files" || arg == "-f" {
			applyFiles = true
		}
		_ = i
	}

	body := map[string]interface{}{
		"snapshot_id": snapshotID,
		"apply_db":    applyDB,
		"apply_files": applyFiles,
	}

	fmt.Printf("Restoring snapshot %s...\n", snapshotID)
	result, err := client.do("POST", "/restore", body)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdUpgrade(client *CLIClient, args []string) {
	autoRollback := true
	for _, arg := range args {
		if arg == "--no-rollback" || arg == "-n" {
			autoRollback = false
		}
	}

	body := map[string]interface{}{
		"auto_rollback": autoRollback,
	}

	fmt.Println("Initiating WordPress upgrade...")
	result, err := client.do("POST", "/upgrade", body)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdListBackups(client *CLIClient) {
	fmt.Println("Listing backups...")
	result, err := client.do("GET", "/backups", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdListSnapshots(client *CLIClient) {
	fmt.Println("Listing restic snapshots...")
	result, err := client.do("GET", "/snapshots", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdHealthcheck(client *CLIClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: wp-maintenance healthcheck <url>")
		os.Exit(1)
	}

	url := args[0]
	body := map[string]interface{}{
		"url": url,
	}

	fmt.Printf("Running healthcheck on %s...\n", url)
	result, err := client.do("POST", "/healthcheck", body)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdStatus(client *CLIClient) {
	fmt.Println("Getting system status...")
	result, err := client.do("GET", "/status", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func cmdLogin(client *CLIClient, args []string) {
	fmt.Printf("Password: ")
	var password string
	fmt.Scanln(&password)

	body := map[string]interface{}{
		"password": password,
	}

	result, err := client.do("POST", "/auth/login", body)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if token, ok := data["token"].(string); ok {
			fmt.Printf("\nLogin successful!\n")
			fmt.Printf("Token: %s\n", token)
			fmt.Println("Set it as environment variable:")
			fmt.Printf("  export WP_MAINTENANCE_TOKEN=%s\n", token)
		}
	} else {
		printJSON(result)
	}
}

func cmdUser(client *CLIClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage:")
		fmt.Println("  wp-maintenance user list")
		fmt.Println("  wp-maintenance user create <username> <password> <role>")
		fmt.Println("  wp-maintenance user delete <user_id>")
		fmt.Println("  wp-maintenance user change-password <user_id> <new_password>")
		os.Exit(1)
	}

	subcommand := args[0]

	switch subcommand {
	case "list":
		result, err := client.do("GET", "/users", nil)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "create":
		if len(args) < 4 {
			fmt.Println("Usage: wp-maintenance user create <username> <password> <role>")
			os.Exit(1)
		}
		body := map[string]interface{}{
			"username": args[1],
			"password": args[2],
			"role":     args[3],
		}
		result, err := client.do("POST", "/users", body)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "delete":
		if len(args) < 2 {
			fmt.Println("Usage: wp-maintenance user delete <user_id>")
			os.Exit(1)
		}
		result, err := client.do("DELETE", "/users/"+args[1], nil)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "change-password":
		if len(args) < 3 {
			fmt.Println("Usage: wp-maintenance user change-password <user_id> <new_password>")
			os.Exit(1)
		}
		body := map[string]interface{}{
			"user_id":      args[1],
			"new_password": args[2],
		}
		result, err := client.do("POST", "/auth/change-password", body)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	default:
		fmt.Printf("Unknown user subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func cmdToken(client *CLIClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage:")
		fmt.Println("  wp-maintenance token list")
		fmt.Println("  wp-maintenance token create <user_id> <name> [duration_hours]")
		fmt.Println("  wp-maintenance token revoke <token_id>")
		os.Exit(1)
	}

	subcommand := args[0]

	switch subcommand {
	case "list":
		result, err := client.do("GET", "/tokens", nil)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "create":
		if len(args) < 3 {
			fmt.Println("Usage: wp-maintenance token create <user_id> <name> [duration_hours]")
			os.Exit(1)
		}
		duration := 0
		if len(args) >= 4 {
			fmt.Sscanf(args[3], "%d", &duration)
		}
		body := map[string]interface{}{
			"user_id":  args[1],
			"name":     args[2],
			"duration": duration,
		}
		result, err := client.do("POST", "/tokens", body)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	case "revoke":
		if len(args) < 2 {
			fmt.Println("Usage: wp-maintenance token revoke <token_id>")
			os.Exit(1)
		}
		result, err := client.do("DELETE", "/tokens/"+args[1], nil)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)

	default:
		fmt.Printf("Unknown token subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func cmdConfig(client *CLIClient, args []string) {
	if len(args) >= 1 && args[0] == "update" {
		var body map[string]interface{}
		if err := json.Unmarshal([]byte(strings.Join(args[1:], " ")), &body); err != nil {
			fmt.Printf("Error: invalid JSON: %v\n", err)
			os.Exit(1)
		}
		result, err := client.do("PUT", "/config", body)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		printJSON(result)
		return
	}

	result, err := client.do("GET", "/config", nil)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	printJSON(result)
}

func printJSON(data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting output: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

func cmdResetPassword(client *CLIClient, args []string) {
	fmt.Printf("Enter new password: ")
	var password string
	fmt.Scanln(&password)

	fmt.Printf("Confirm new password: ")
	var passwordConfirm string
	fmt.Scanln(&passwordConfirm)

	if password != passwordConfirm {
		fmt.Println("Passwords do not match")
		os.Exit(1)
	}

	if err := auth.ValidatePassword(password); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	dbPath := filepath.Join(dataDir, "wp-maintenance.db")

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		secretKey = "default-secret-key"
	}

	authMgr, err := auth.NewAuthManager(dbPath, secretKey)
	if err != nil {
		fmt.Printf("Error: failed to initialize auth manager for %s: %v\n", dbPath, err)
		os.Exit(1)
	}

	admin, err := authMgr.GetAdminUser()
	if err != nil {
		fmt.Printf("Error: failed to find internal admin user in %s: %v\n", dbPath, err)
		os.Exit(1)
	}

	if err := authMgr.ChangePassword(admin.ID, password); err != nil {
		fmt.Printf("Error: failed to reset password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Password reset successful for user '%s' using local database '%s'.\n", admin.Username, dbPath)
}
