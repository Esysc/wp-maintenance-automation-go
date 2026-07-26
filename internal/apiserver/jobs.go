package apiserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/restic"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/upgrade"
)

const jobQueueSize = 100

type jobRequest struct {
	SiteID         string `json:"site_id"`
	SnapshotID     string `json:"snapshot_id"`
	AutoRollback   bool   `json:"auto_rollback"`
	HealthcheckURL string `json:"healthcheck_url"`
	ApplyDB        bool   `json:"apply_db"`
	ApplyFiles     bool   `json:"apply_files"`
	Confirm        bool   `json:"confirm"`
}

func (s *APIServer) enqueueJob(jobType string, req jobRequest) (*db.Job, error) {
	job := &db.Job{
		ID:     auth.GenerateID(),
		Type:   jobType,
		SiteID: req.SiteID,
		Status: "queued",
	}
	if err := s.Database.CreateJob(job); err != nil {
		return nil, err
	}
	select {
	case s.jobQueue <- &jobTuple{job: job, req: req}:
	default:
		return nil, fmt.Errorf("job queue is full")
	}
	return job, nil
}

type jobTuple struct {
	job *db.Job
	req jobRequest
}

func (s *APIServer) startWorker() {
	s.workerOnce.Do(func() {
		go s.workerLoop()
		log.Printf("background job worker started")
	})
}

func (s *APIServer) workerLoop() {
	for tuple := range s.jobQueue {
		s.processJob(tuple)
	}
}

func (s *APIServer) processJob(tuple *jobTuple) {
	job := tuple.job

	setStatus := func(status, progress string) {
		job.Status = status
		job.Progress = progress
		s.Database.UpdateJobStatus(job.ID, status, progress, "", "")
	}
	setError := func(errMsg string) {
		job.Status = "failed"
		job.Error = errMsg
		s.Database.UpdateJobStatus(job.ID, "failed", "", "", errMsg)
	}
	setResult := func(result string) {
		job.Status = "completed"
		job.Result = result
		s.Database.UpdateJobStatus(job.ID, "completed", "", result, "")
	}
	cleanupRemoveAll := func(path string) {
		if err := os.RemoveAll(path); err != nil {
			log.Printf("cleanup error removing %s: %v", path, err)
		}
	}

	setStatus("running", "loading site configuration")

	site, err := s.Database.GetSite(job.SiteID)
	if err != nil {
		setError("site not found: " + err.Error())
		return
	}

	sshOpts := ssh.NewSSHOptions(site.WPSSHHost, site.WPSSHUser, site.WPSSHPort)
	sshOpts.Key = site.WPSSHKey
	sshClient := ssh.NewClient(sshOpts)
	defer sshClient.Close()

	wpRoot := site.WPRoot

	switch job.Type {
	case "backup":
		setStatus("running", "creating backup directories")
		timestamp := time.Now().Format("20060102_150405")
		backupDir := site.BackupDir
		if backupDir == "" {
			backupDir = "./backups"
		}
		workDir := filepath.Join(backupDir, timestamp)
		dbDir := filepath.Join(workDir, "db")
		wpDir := filepath.Join(workDir, "wp")
		if err := os.MkdirAll(dbDir, 0700); err != nil {
			setError("failed to create work directory: " + err.Error())
			return
		}
		if err := os.MkdirAll(wpDir, 0700); err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to create wp directory: " + err.Error())
			return
		}

		if wpRoot == "" {
			setStatus("running", "detecting WordPress root")
			detectedRoot, err := sshClient.DetectWPRoot()
			if err != nil {
				cleanupRemoveAll(workDir)
				setError("could not detect WordPress root: " + err.Error())
				return
			}
			wpRoot = detectedRoot
		}

		setStatus("running", "getting WordPress version")
		wpVersion, _ := sshClient.WPCLI(wpRoot, "core version")

		setStatus("running", "parsing database configuration")
		dbName, dbUser, dbPassword, dbHost, err := sshClient.ParseDBConfig(wpRoot)
		if err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to parse DB config: " + err.Error())
			return
		}
		if dbHost == "" {
			dbHost = "localhost"
		}

		setStatus("running", "dumping database")
		dumpOutput, err := sshClient.RunCommand(fmt.Sprintf(
			"mysqldump --single-transaction --quick --lock-tables=false -u%s -p'%s' -h%s %s",
			dbUser, dbPassword, dbHost, dbName,
		))
		if err != nil {
			cleanupRemoveAll(workDir)
			setError("database dump failed: " + err.Error())
			return
		}
		dumpFile := filepath.Join(dbDir, timestamp+"_"+dbName+".sql")
		if err := os.WriteFile(dumpFile, []byte(dumpOutput), 0600); err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to write db dump: " + err.Error())
			return
		}

		setStatus("running", "syncing WordPress files")
		rsyncExcludes := []string{"wp-content/cache/"}
		if err := sshClient.SyncDir(sshOpts.Host+":"+wpRoot+"/", wpDir+"/", rsyncExcludes, false); err != nil {
			cleanupRemoveAll(workDir)
			setError("file sync failed: " + err.Error())
			return
		}

		fileCount := 0
		filepath.Walk(wpDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				fileCount++
			}
			return nil
		})

		resticRepo := site.ResticRepository
		resticPassFile := site.ResticPasswordFile
		if resticRepo == "" && s.Restic != nil {
			resticRepo = s.Restic.Repository
		}
		if resticPassFile == "" && s.Restic != nil {
			resticPassFile = s.Restic.PasswordFile
		}
		if resticRepo == "" {
			cleanupRemoveAll(workDir)
			setError("no restic repository configured")
			return
		}

		setStatus("running", "creating restic backup")
		resticClient, err := restic.NewClient(resticRepo, resticPassFile)
		if err != nil {
			cleanupRemoveAll(workDir)
			setError("restic configuration error")
			return
		}

		resticTags := []string{site.ID, site.Name, "backup:" + timestamp}
		if _, err := resticClient.Backup(workDir, resticTags); err != nil {
			cleanupRemoveAll(workDir)
			setError("restic backup failed: " + err.Error())
			return
		}

		if site.RetentionFlags != "" {
			flags := strings.Fields(site.RetentionFlags)
			if len(flags) > 0 {
				resticClient.Forget(flags...)
			}
		}

		backupRecord := &db.Backup{
			ID:        auth.GenerateID(),
			SiteID:    job.SiteID,
			Timestamp: timestamp,
			Host:      site.WPSSHHost,
			WPRoot:    wpRoot,
			WPVersion: wpVersion,
			DBName:    dbName,
			DBHost:    dbHost,
			DumpFile:  dumpFile,
			FileCount: fileCount,
		}
		if err := s.Database.CreateBackup(backupRecord); err != nil {
			setError("failed to record backup: " + err.Error())
			return
		}

		log.Printf("backup completed for site %s: backup_id=%s timestamp=%s", site.Name, backupRecord.ID, timestamp)
		setResult(fmt.Sprintf(`{"backup_id":"%s","timestamp":"%s"}`, backupRecord.ID, timestamp))

	case "restore":
		if wpRoot == "" {
			setStatus("running", "detecting WordPress root")
			detectedRoot, err := sshClient.DetectWPRoot()
			if err != nil {
				setError("could not detect WordPress root: " + err.Error())
				return
			}
			wpRoot = detectedRoot
		}

		resticRepo := site.ResticRepository
		resticPassFile := site.ResticPasswordFile
		if resticRepo == "" && s.Restic != nil {
			resticRepo = s.Restic.Repository
		}
		if resticPassFile == "" && s.Restic != nil {
			resticPassFile = s.Restic.PasswordFile
		}
		if resticRepo == "" {
			setError("no restic repository configured")
			return
		}

		setStatus("running", "configuring restic client")
		resticClient, err := restic.NewClient(resticRepo, resticPassFile)
		if err != nil {
			setError("restic configuration error")
			return
		}

		restoreDir, err := os.MkdirTemp("", "wpmaintenance-restore-*")
		if err != nil {
			setError("failed to create restore directory: " + err.Error())
			return
		}
		defer cleanupRemoveAll(restoreDir)

		setStatus("running", "restoring from snapshot "+tuple.req.SnapshotID)
		if err := resticClient.Restore(tuple.req.SnapshotID, restoreDir); err != nil {
			setError("restic restore failed: " + err.Error())
			return
		}

		if tuple.req.ApplyDB {
			setStatus("running", "restoring database")
			var dbDump string
			filepath.Walk(restoreDir, func(path string, info os.FileInfo, _ error) error {
				if info != nil && !info.IsDir() && strings.HasSuffix(path, ".sql") {
					dbDump = path
					return fmt.Errorf("found")
				}
				return nil
			})
			if dbDump == "" {
				filepath.Walk(restoreDir, func(path string, info os.FileInfo, _ error) error {
					if info != nil && !info.IsDir() && strings.HasSuffix(path, ".sql.gz") {
						dbDump = path
						return fmt.Errorf("found")
					}
					return nil
				})
			}
			if dbDump == "" {
				setError("no database dump found in snapshot")
				return
			}

			dbName, dbUser, dbPassword, dbHost, err := sshClient.ParseDBConfig(wpRoot)
			if err != nil {
				setError("failed to parse DB config: " + err.Error())
				return
			}
			if dbHost == "" {
				dbHost = "localhost"
			}

			remoteDump := "/tmp/wp-restore-" + tuple.req.SnapshotID + ".sql"
			if strings.HasSuffix(dbDump, ".gz") {
				remoteDump += ".gz"
			}

			if err := sshClient.UploadFile(dbDump, remoteDump); err != nil {
				setError("failed to upload db dump: " + err.Error())
				return
			}

			var importCmd string
			if strings.HasSuffix(dbDump, ".gz") {
				importCmd = fmt.Sprintf("gunzip -c %s | mysql -u%s -p'%s' -h%s %s && rm -f %s",
					remoteDump, dbUser, dbPassword, dbHost, dbName, remoteDump)
			} else {
				importCmd = fmt.Sprintf("mysql -u%s -p'%s' -h%s %s < %s && rm -f %s",
					dbUser, dbPassword, dbHost, dbName, remoteDump, remoteDump)
			}
			if _, err := sshClient.RunCommand(importCmd); err != nil {
				setError("database restore failed: " + err.Error())
				return
			}
		}

		if tuple.req.ApplyFiles {
			setStatus("running", "restoring WordPress files")
			var wpDir string
			filepath.Walk(restoreDir, func(path string, info os.FileInfo, _ error) error {
				if info != nil && info.IsDir() && (info.Name() == "wp" || info.Name() == "wordpress" || info.Name() == "html") {
					wpDir = path
					return fmt.Errorf("found")
				}
				return nil
			})
			if wpDir == "" {
				wpDir = restoreDir
			}

			rsyncExcludes := []string{"wp-content/cache/"}
			remoteDest := sshOpts.Host + ":" + wpRoot + "/"
			if err := sshClient.SyncDir(wpDir+"/", remoteDest, rsyncExcludes, true); err != nil {
				setError("file restore failed: " + err.Error())
				return
			}
		}

		log.Printf("restore completed for site %s: snapshot=%s", site.Name, tuple.req.SnapshotID)
		setResult(fmt.Sprintf(`{"snapshot_id":"%s"}`, tuple.req.SnapshotID))

	case "upgrade":
		if wpRoot == "" {
			setStatus("running", "detecting WordPress root")
			detectedRoot, err := sshClient.DetectWPRoot()
			if err != nil {
				setError("could not detect WordPress root: " + err.Error())
				return
			}
			wpRoot = detectedRoot
		}

		healthcheckURL := tuple.req.HealthcheckURL
		if healthcheckURL == "" {
			healthcheckURL = site.HealthcheckURL
		}

		setStatus("running", "running WordPress upgrade")
		um := upgrade.NewUpgradeManager(&upgrade.UpgradeOptions{
			SSHClient:        sshClient,
			WPRoot:           wpRoot,
			HealthcheckURL:   healthcheckURL,
			AutoRollback:     tuple.req.AutoRollback,
			StagingRehearsal: site.StagingEnabled,
			StagingManager:   s.StagingManager,
			SnapshotID:       tuple.req.SnapshotID,
			ReportDir:        site.BackupDir,
		})

		report, err := um.Run()
		if err != nil {
			setError("upgrade failed: " + err.Error())
			return
		}

		status := "completed"
		if report.FinalStatus == "failed" {
			status = "failed"
		}

		log.Printf("upgrade %s for site %s", status, site.Name)
		reportJSON, _ := json.Marshal(report)
		setResult(fmt.Sprintf(`{"status":"%s","report":%s}`, status, string(reportJSON)))

	default:
		setError("unknown job type: " + job.Type)
	}
}

func (s *APIServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jobs, err := s.Database.ListJobs()
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to list jobs")
		return
	}
	jsonResp(w, http.StatusOK, jobs)
}

func (s *APIServer) handleJobByID(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	job, err := s.Database.GetJob(jobID)
	if err != nil {
		apiErr(w, http.StatusNotFound, "job not found")
		return
	}
	jsonResp(w, http.StatusOK, job)
}
