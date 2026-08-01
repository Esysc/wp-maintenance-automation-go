package webserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WebServer struct {
	apiBaseURL string
	apiToken   string
	client     *http.Client
	staticDir  string
}

func Run() {
	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "8080"
	}

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8081"
	}

	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./web/static"
	}

	s := &WebServer{
		apiBaseURL: apiURL,
		apiToken:   os.Getenv("WP_MAINTENANCE_TOKEN"),
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		staticDir: staticDir,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(staticDir, "favicon.svg"))
	})
	mux.HandleFunc("/api/login", s.handleAPILogin)
	mux.HandleFunc("/api/auth/state", s.handleAPIAuthState)
	mux.HandleFunc("/api/backup", s.authMiddleware(s.handleAPIBackup))
	mux.HandleFunc("/api/backups", s.authMiddleware(s.handleAPIBackups))
	mux.HandleFunc("/api/backups/", s.authMiddleware(s.handleAPIBackups))
	mux.HandleFunc("/api/upgrade", s.authMiddleware(s.handleAPIUpgrade))
	mux.HandleFunc("/api/healthcheck", s.authMiddleware(s.handleAPIHealthcheck))
	mux.HandleFunc("/api/users", s.authMiddleware(s.handleAPIUsers))
	mux.HandleFunc("/api/tokens", s.authMiddleware(s.handleAPITokens))
	mux.HandleFunc("/api/sites", s.authMiddleware(s.handleAPISites))
	mux.HandleFunc("/api/snapshots", s.authMiddleware(s.handleAPISnapshots))
	mux.HandleFunc("/api/v1/auth/state", s.handleAPIAuthState)
	mux.HandleFunc("/api/v1/status", s.handleAPIStatus)
	mux.HandleFunc("/api/v1/health", s.handleAPIHealth)
	mux.HandleFunc("/api/v1/backup", s.authMiddleware(s.handleAPIBackup))
	mux.HandleFunc("/api/v1/backups", s.authMiddleware(s.handleAPIBackups))
	mux.HandleFunc("/api/v1/backups/", s.authMiddleware(s.handleAPIBackups))
	mux.HandleFunc("/api/v1/upgrade", s.authMiddleware(s.handleAPIUpgrade))
	mux.HandleFunc("/api/v1/restore", s.authMiddleware(s.handleAPIRestore))
	mux.HandleFunc("/api/v1/healthcheck", s.authMiddleware(s.handleAPIHealthcheck))
	mux.HandleFunc("/api/v1/users", s.authMiddleware(s.handleAPIUsers))
	mux.HandleFunc("/api/v1/users/", s.authMiddleware(s.handleAPIUsers))
	mux.HandleFunc("/api/v1/tokens", s.authMiddleware(s.handleAPITokens))
	mux.HandleFunc("/api/v1/tokens/", s.authMiddleware(s.handleAPITokens))
	mux.HandleFunc("/api/v1/sites/detect-config", s.authMiddleware(s.handleAPIDetectConfig))
	mux.HandleFunc("/api/v1/sites", s.authMiddleware(s.handleAPISites))
	mux.HandleFunc("/api/v1/sites/", s.authMiddleware(s.handleAPISites))
	mux.HandleFunc("/api/v1/snapshots", s.authMiddleware(s.handleAPISnapshots))
	mux.HandleFunc("/api/v1/snapshots/", s.authMiddleware(s.handleAPISnapshots))
	mux.HandleFunc("/api/v1/rehearsal", s.authMiddleware(s.handleAPIRehearsal))
	mux.HandleFunc("/api/v1/rehearsal/", s.authMiddleware(s.handleAPIRehearsalByID))
	mux.HandleFunc("/api/v1/jobs", s.authMiddleware(s.handleAPIJobs))
	mux.HandleFunc("/api/v1/jobs/", s.authMiddleware(s.handleAPIJobs))
	mux.HandleFunc("/api/v1/metrics", s.authMiddleware(s.handleAPIMetrics))
	mux.HandleFunc("/api/docs/", s.handleDocs)

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
	mux.Handle("/ui/assets/", http.StripPrefix("/ui/", http.FileServer(http.Dir(filepath.Join(staticDir, "ui")))))
	mux.HandleFunc("/ui/", s.handleUISPA)
	mux.HandleFunc("/ui", s.handleUISPA)

	tlsDisable := os.Getenv("WEB_TLS_DISABLE")

	if tlsDisable == "true" {
		log.Printf("Web server running on http://localhost:%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		log.Printf("Web server running on http://localhost:%s", port)
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

func generateSelfSignedCert() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %w", err)
	}

	return &cert, nil
}

func (s *WebServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie("token"); err == nil {
			token = c.Value
		}
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			http.Error(w, `{"success":false,"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}

		req, _ := http.NewRequest("GET", s.apiBaseURL+"/api/v1/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := s.client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			http.Error(w, `{"success":false,"error":"invalid or expired token"}`, http.StatusUnauthorized)
			return
		}
		resp.Body.Close()

		next(w, r)
	}
}

func (s *WebServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.staticDir, "ui", "index.html"))
}

func (s *WebServer) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var loginReq struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &loginReq); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	apiReq, _ := http.NewRequest("POST", s.apiBaseURL+"/api/v1/auth/login", strings.NewReader(string(body)))
	apiReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(apiReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var apiResult map[string]interface{}
	json.Unmarshal(respBody, &apiResult)

	if resp.StatusCode == 200 {
		if data, ok := apiResult["data"].(map[string]interface{}); ok {
			if token, ok := data["token"].(string); ok {
				http.SetCookie(w, &http.Cookie{
					Name:    "token",
					Value:   token,
					Path:    "/",
					Expires: time.Now().Add(24 * time.Hour),
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	statusCode := resp.StatusCode
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadRequest {
		// Return 200 for expected login failures so browser console is not polluted
		// with failed-resource network errors. The JSON payload still carries success=false.
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

func (s *WebServer) handleAPIAuthState(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/auth/state")
}

func (s *WebServer) handleAPIBackup(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/backup")
}

func (s *WebServer) handleAPIUpgrade(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/upgrade")
}

func (s *WebServer) handleAPIRestore(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/restore")
}

func (s *WebServer) handleAPIHealthcheck(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/healthcheck")
}

func (s *WebServer) handleAPIDetectConfig(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/sites/detect-config")
}

func (s *WebServer) handleAPIUsers(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/users")
}

func (s *WebServer) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api/v1/tokens" || path == "/api/v1/tokens/" {
		if r.Method == "GET" || r.Method == "POST" {
			s.proxyRequest(w, r, "/api/v1/tokens")
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		return
	}

	if strings.HasPrefix(path, "/api/v1/tokens/") {
		tokenID := strings.TrimPrefix(path, "/api/v1/tokens/")
		if r.Method == "DELETE" {
			s.proxyRequestWithID(w, r, "/api/v1/tokens/", tokenID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	} else {
		if r.Method == "GET" || r.Method == "POST" {
			s.proxyRequest(w, r, "/api/v1/tokens")
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}
}

func (s *WebServer) handleAPIBackups(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/api/v1/backups" || path == "/api/v1/backups/" || path == "/api/backups" || path == "/api/backups/" {
		if r.Method == "GET" {
			s.proxyRequest(w, r, "/api/v1/backups")
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasPrefix(path, "/api/v1/backups/") {
		backupID := strings.TrimPrefix(path, "/api/v1/backups/")
		if r.Method == "GET" || r.Method == "DELETE" {
			s.proxyRequestWithID(w, r, "/api/v1/backups/", backupID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasPrefix(path, "/api/backups/") {
		backupID := strings.TrimPrefix(path, "/api/backups/")
		if r.Method == "GET" || r.Method == "DELETE" {
			s.proxyRequestWithID(w, r, "/api/v1/backups/", backupID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *WebServer) handleAPISites(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api/v1/sites" || path == "/api/v1/sites/" {
		if r.Method == "GET" || r.Method == "POST" {
			s.proxyRequest(w, r, "/api/v1/sites")
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		return
	}

	if strings.HasPrefix(path, "/api/v1/sites/") {
		siteID := strings.TrimPrefix(path, "/api/v1/sites/")
		if r.Method == "GET" || r.Method == "PUT" || r.Method == "DELETE" {
			s.proxyRequestWithID(w, r, "/api/v1/sites/", siteID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	} else {
		if r.Method == "GET" || r.Method == "POST" {
			s.proxyRequest(w, r, "/api/v1/sites")
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	}
}

func (s *WebServer) handleAPISnapshots(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/snapshots")
}

func (s *WebServer) handleAPIRehearsal(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/rehearsal")
}

func (s *WebServer) handleAPIRehearsalByID(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, r.URL.Path)
}

func (s *WebServer) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/api/v1/jobs" || path == "/api/v1/jobs/" {
		s.proxyRequest(w, r, "/api/v1/jobs")
		return
	}

	if strings.HasPrefix(path, "/api/v1/jobs/") {
		jobID := strings.TrimPrefix(path, "/api/v1/jobs/")
		if r.Method == "GET" || r.Method == "DELETE" || r.Method == "POST" {
			s.proxyRequestWithID(w, r, "/api/v1/jobs/", jobID)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (s *WebServer) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/status")
}

func (s *WebServer) handleAPIHealth(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/health")
}

func (s *WebServer) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	s.proxyRequest(w, r, "/api/v1/metrics")
}

func (s *WebServer) proxyAuth(apiReq *http.Request, r *http.Request) {
	if auth := r.Header.Get("Authorization"); auth != "" {
		apiReq.Header.Set("Authorization", auth)
		return
	}
	if c, err := r.Cookie("token"); err == nil {
		apiReq.Header.Set("Authorization", "Bearer "+c.Value)
	}
}

func (s *WebServer) proxyRequest(w http.ResponseWriter, r *http.Request, path string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	apiURL := s.apiBaseURL + path
	if r.URL.RawQuery != "" {
		apiURL += "?" + r.URL.RawQuery
	}

	apiReq, _ := http.NewRequest(r.Method, apiURL, strings.NewReader(string(body)))
	apiReq.Header.Set("Content-Type", "application/json")

	s.proxyAuth(apiReq, r)

	resp, err := s.client.Do(apiReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (s *WebServer) proxyRequestWithID(w http.ResponseWriter, r *http.Request, basePath, id string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	apiPath := basePath + id
	if r.URL.RawQuery != "" {
		apiPath += "?" + r.URL.RawQuery
	}

	apiReq, _ := http.NewRequest(r.Method, s.apiBaseURL+apiPath, strings.NewReader(string(body)))
	apiReq.Header.Set("Content-Type", "application/json")

	s.proxyAuth(apiReq, r)

	resp, err := s.client.Do(apiReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

func (s *WebServer) handleUISPA(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(s.staticDir, "ui", "index.html"))
}

func (s *WebServer) handleDocs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/docs/")
	if path == "" || path == "/" {
		path = "index.html"
	}

	docsPath := filepath.Join("api/docs", path)
	if !strings.HasPrefix(docsPath, "api/docs") {
		http.Error(w, "invalid path", http.StatusForbidden)
		return
	}

	if docsPath == "api/docs/" || docsPath == "api/docs" || docsPath == "api/docs/index.html" {
		docsPath = "api/docs/index.html"
	}
	http.ServeFile(w, r, docsPath)
}
