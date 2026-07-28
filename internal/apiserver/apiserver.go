package apiserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/healthcheck"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/restic"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/staging"
)

type APIServer struct {
	Auth           *auth.AuthManager
	Database       *db.Database
	Checker        *healthcheck.Checker
	Restic         *restic.ResticClient
	SSHClient      *ssh.Client
	StagingManager *staging.Manager
	jobQueue       chan *jobTuple
	workerOnce     sync.Once
}

func Run() {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8081"
	}

	connStr := buildDBConnStr()
	database, err := db.New(connStr)
	if err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		secretKey = "default-secret-key"
	}

	authMgr := auth.NewAuthManager(database, secretKey)

	checker := healthcheck.NewChecker()

	var resticClient *restic.ResticClient
	resticRepository := os.Getenv("RESTIC_REPOSITORY")
	resticPasswordFile := os.Getenv("RESTIC_PASSWORD_FILE")
	if resticRepository == "" {
		resticRepository = "/app/backup_artifacts/restic-repo"
	}
	if resticPasswordFile == "" {
		resticPasswordFile = filepath.Join(dataDir, "restic_password")
	}

	if _, statErr := os.Stat(resticPasswordFile); os.IsNotExist(statErr) {
		pass := os.Getenv("RESTIC_PASSWORD")
		if pass == "" {
			pass = "wp-maintenance-local-password"
			log.Printf("RESTIC_PASSWORD is not set; using local development default password file")
		}
		if mkErr := os.MkdirAll(filepath.Dir(resticPasswordFile), 0700); mkErr == nil {
			if writeErr := os.WriteFile(resticPasswordFile, []byte(pass), 0600); writeErr != nil {
				log.Printf("Restic password file setup failed: %v", writeErr)
			}
		}
	}

	resticClient, err = restic.NewClient(resticRepository, resticPasswordFile)
	if err != nil {
		log.Printf("Restic disabled: %v", err)
	} else {
		// On first run for local path repositories, initialize restic metadata.
		if !strings.Contains(resticRepository, ":") {
			if mkErr := os.MkdirAll(resticRepository, 0700); mkErr != nil {
				log.Printf("Restic repository directory setup failed: %v", mkErr)
			} else if _, cfgErr := os.Stat(filepath.Join(resticRepository, "config")); os.IsNotExist(cfgErr) {
				if initErr := resticClient.Init(); initErr != nil {
					log.Printf("Restic init warning: %v", initErr)
				}
			}
		}
	}

	s := &APIServer{
		Auth:           authMgr,
		Database:       database,
		Checker:        checker,
		Restic:         resticClient,
		StagingManager: staging.NewManager("./staging"),
		jobQueue:       make(chan *jobTuple, jobQueueSize),
	}

	s.startWorker()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/status", s.handleStatus)

	mux.HandleFunc("/api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/api/v1/auth/state", s.handleAuthState)
	mux.HandleFunc("/api/v1/auth/change-password", s.authMiddleware(s.handleChangePassword))

	mux.HandleFunc("/api/v1/users", s.authMiddleware(s.handleUsers))
	mux.HandleFunc("/api/v1/users/", s.authMiddleware(s.handleUserByID))

	mux.HandleFunc("/api/v1/tokens", s.authMiddleware(s.handleTokens))
	mux.HandleFunc("/api/v1/tokens/", s.authMiddleware(s.handleTokenByID))

	mux.HandleFunc("/api/v1/sites", s.authMiddleware(s.handleSites))
	mux.HandleFunc("/api/v1/sites/detect-config", s.authMiddleware(s.handleDetectConfig))
	mux.HandleFunc("/api/v1/sites/", s.authMiddleware(s.handleSiteByID))

	mux.HandleFunc("/api/v1/backups", s.authMiddleware(s.handleBackups))
	mux.HandleFunc("/api/v1/backups/", s.authMiddleware(s.handleBackupByID))
	mux.HandleFunc("/api/v1/backup", s.authMiddleware(s.handleBackup))
	mux.HandleFunc("/api/v1/restore", s.authMiddleware(s.handleRestore))
	mux.HandleFunc("/api/v1/upgrade", s.authMiddleware(s.handleUpgrade))

	mux.HandleFunc("/api/v1/snapshots", s.authMiddleware(s.handleSnapshots))
	mux.HandleFunc("/api/v1/snapshots/", s.authMiddleware(s.handleSnapshotByID))

	mux.HandleFunc("/api/v1/healthcheck", s.authMiddleware(s.handleHealthcheck))

	mux.HandleFunc("/api/v1/staging/cleanup", s.authMiddleware(s.handleStagingCleanup))

	mux.HandleFunc("/api/v1/rehearsal", s.authMiddleware(s.handleRehearsal))
	mux.HandleFunc("/api/v1/rehearsal/", s.authMiddleware(s.handleRehearsalByID))

	mux.HandleFunc("/api/v1/jobs", s.authMiddleware(s.handleJobs))
	mux.HandleFunc("/api/v1/jobs/", s.authMiddleware(s.handleJobByID))

	handler := corsMiddleware(mux)

	tlsDisable := os.Getenv("TLS_DISABLE")

	if tlsDisable == "true" {
		log.Printf("API server running on http://localhost:%s", port)
		if err := http.ListenAndServe(":"+port, handler); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		log.Printf("API server running on http://localhost:%s", port)
		if err := http.ListenAndServe(":"+port, handler); err != nil {
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

func buildDBConnStr() string {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	if user == "" {
		user = "wpmaint"
	}
	if password == "" {
		password = "wpmaint"
	}
	if dbname == "" {
		dbname = "wpmaintenance"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
}
