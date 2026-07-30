package apiserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
	reqJSON, _ := json.Marshal(req)
	job := &db.Job{
		ID:              auth.GenerateID(),
		Type:            jobType,
		SiteID:          req.SiteID,
		Status:          "queued",
		Progress:        "job_progress_queued",
		ProgressPercent: 0,
		Result:          string(reqJSON),
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
		s.recoverQueuedJobs()
		log.Printf("background job worker started")
	})
}

func (s *APIServer) recoverQueuedJobs() {
	jobs, err := s.Database.ListQueuedJobs()
	if err != nil {
		log.Printf("failed to recover queued jobs: %v", err)
		return
	}
	for _, j := range jobs {
		req := jobRequest{SiteID: j.SiteID}
		if j.Result != "" {
			json.Unmarshal([]byte(j.Result), &req)
		}
		s.jobQueue <- &jobTuple{job: j, req: req}
		log.Printf("recovered queued job %s (%s)", j.ID, j.Type)
	}
}

func (s *APIServer) workerLoop() {
	for tuple := range s.jobQueue {
		s.processJob(tuple)
	}
}

func (s *APIServer) processJob(tuple *jobTuple) {
	job := tuple.job
	log.Printf("processing job %s (%s)", job.ID, job.Type)

	condenseError := func(errMsg string) string {
		errMsg = strings.TrimSpace(errMsg)
		if errMsg == "" {
			return "unknown error"
		}
		if idx := strings.Index(errMsg, "\n"); idx >= 0 {
			errMsg = errMsg[:idx]
		}
		if len(errMsg) > 320 {
			errMsg = errMsg[:320] + "..."
		}
		return errMsg
	}

	setStatus := func(status, progress string, percent int) {
		job.Status = status
		job.Progress = progress
		job.ProgressPercent = percent
		s.Database.UpdateJobStatus(job.ID, status, progress, percent, "", "")
	}
	setError := func(errMsg string) {
		job.Status = "failed"
		errMsg = condenseError(errMsg)
		job.Error = errMsg
		s.Database.UpdateJobStatus(job.ID, "failed", job.Progress, job.ProgressPercent, "", errMsg)
	}
	setResult := func(result string) {
		job.Status = "completed"
		job.Progress = "job_progress_completed"
		job.ProgressPercent = 100
		job.Result = result
		s.Database.UpdateJobStatus(job.ID, "completed", job.Progress, job.ProgressPercent, result, "")
	}
	isCancelled := func() bool {
		current, err := s.Database.GetJob(job.ID)
		if err != nil {
			return false
		}
		return current.Status == "cancelled"
	}
	cleanupRemoveAll := func(path string) {
		if err := os.RemoveAll(path); err != nil {
			log.Printf("cleanup error removing %s: %v", path, err)
		}
	}

	setStatus("running", "job_progress_loading_site_configuration", 5)

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
		setStatus("running", "job_progress_backup_creating_directories", 10)
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
			setStatus("running", "job_progress_detecting_wp_root", 15)
			detectedRoot, err := sshClient.DetectWPRoot()
			if err != nil {
				cleanupRemoveAll(workDir)
				setError("could not detect WordPress root: " + err.Error())
				return
			}
			wpRoot = detectedRoot
		}

		setStatus("running", "job_progress_backup_reading_wp_version", 20)
		wpVersion, err := sshClient.GetWpVersion(wpRoot)
		if err != nil {
			log.Printf("warning: could not detect WordPress version: %v", err)
		}

		setStatus("running", "job_progress_backup_parsing_db_config", 30)
		dbName, dbUser, dbPassword, dbHost, err := sshClient.ParseDBConfig(wpRoot)
		if err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to parse DB config: " + err.Error())
			return
		}
		if dbHost == "" {
			dbHost = "localhost"
		}

		setStatus("running", "job_progress_backup_dumping_database", 45)
		shellQuote := func(value string) string {
			return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
		}
		remoteCredFile := "/tmp/wpma-mysqldump-" + timestamp + ".cnf"
		defer sshClient.RemoveRemoteFile(remoteCredFile)
		remoteCredContent := fmt.Sprintf("[client]\nuser=%s\npassword=%s\nhost=%s\n", dbUser, dbPassword, dbHost)
		if _, err := sshClient.RunCommand(fmt.Sprintf("umask 077 && cat > %s << 'EOF'\n%sEOF", shellQuote(remoteCredFile), remoteCredContent)); err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to prepare database credentials: " + err.Error())
			return
		}
		remoteDumpFile := "/tmp/wpma-dump-" + timestamp + ".sql"
		defer sshClient.RemoveRemoteFile(remoteDumpFile)
		dumpCmd := fmt.Sprintf(
			"mysqldump --defaults-extra-file=%s --single-transaction --quick --lock-tables=false --no-tablespaces %s > %s 2>/dev/null; [ $? -le 3 ]",
			shellQuote(remoteCredFile), shellQuote(dbName), shellQuote(remoteDumpFile),
		)
		if _, err := sshClient.RunCommand(dumpCmd); err != nil {
			// mysqldump exit 3 = warnings (non-fatal). Check if the dump file has content.
			if checkErr := sshClient.RunCommandRaw(fmt.Sprintf("test -s %s", shellQuote(remoteDumpFile))); checkErr != "" {
				cleanupRemoveAll(workDir)
				setError("database dump failed: " + err.Error())
				return
			}
		}

		dumpFile := filepath.Join(dbDir, timestamp+"_"+dbName+".sql")
		if err := sshClient.DownloadFile(remoteDumpFile, dumpFile); err != nil {
			cleanupRemoveAll(workDir)
			setError("failed to download db dump: " + err.Error())
			return
		}

		if isCancelled() {
			setStatus("cancelled", "job_progress_cancelled", 0)
			cleanupRemoveAll(workDir)
			return
		}

		setStatus("running", "job_progress_backup_syncing_files", 60)
		totalFiles, _ := sshClient.CountRemoteFiles(wpRoot + "/")
		if totalFiles < 1 {
			totalFiles = 1
		}
		fileCount := 0
		rsyncExcludes := []string{"wp-content/cache/"}
		if err := sshClient.SyncDir(wpRoot+"/", wpDir+"/", rsyncExcludes, false, func(relPath string) {
			fileCount++
			pct := 60
			if totalFiles > 0 {
				pct = 60 + int(float64(fileCount)/float64(totalFiles)*15)
			}
			setStatus("running", "Copying: "+relPath, pct)
		}); err != nil {
			cleanupRemoveAll(workDir)
			setError("file sync failed: " + err.Error())
			return
		}

		if isCancelled() {
			setStatus("cancelled", "job_progress_cancelled", 0)
			cleanupRemoveAll(workDir)
			return
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
			cleanupRemoveAll(workDir)
			setError("no restic repository configured")
			return
		}

		setStatus("running", "job_progress_backup_creating_restic", 75)
		resticClient, err := restic.NewClient(resticRepo, resticPassFile)
		if err != nil {
			cleanupRemoveAll(workDir)
			setError("restic configuration error")
			return
		}

		resticTags := []string{site.ID, site.Name, "backup:" + timestamp}
		resticSnap, err := resticClient.Backup(workDir, resticTags)
		if err != nil {
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

		snapshotID := ""
		if resticSnap != nil {
			snapshotID = resticSnap.ShortID
		}

		backupRecord := &db.Backup{
			ID:         auth.GenerateID(),
			SiteID:     job.SiteID,
			Timestamp:  timestamp,
			Host:       site.WPSSHHost,
			WPRoot:     wpRoot,
			WPVersion:  wpVersion,
			DBName:     dbName,
			DBHost:     dbHost,
			DumpFile:   dumpFile,
			FileCount:  fileCount,
			SnapshotID: snapshotID,
		}
		if err := s.Database.CreateBackup(backupRecord); err != nil {
			setError("failed to record backup: " + err.Error())
			return
		}

		setStatus("running", "job_progress_backup_recording_metadata", 90)

		log.Printf("backup completed for site %s: backup_id=%s timestamp=%s", site.Name, backupRecord.ID, timestamp)
		setResult(fmt.Sprintf(`{"backup_id":"%s","timestamp":"%s"}`, backupRecord.ID, timestamp))

	case "restore":
		if wpRoot == "" {
			setStatus("running", "job_progress_detecting_wp_root", 10)
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

		setStatus("running", "job_progress_restore_configuring_restic", 20)
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

		if isCancelled() {
			setStatus("cancelled", "job_progress_cancelled", 0)
			return
		}

		setStatus("running", "job_progress_restore_restoring_snapshot", 35)
		if err := resticClient.Restore(tuple.req.SnapshotID, restoreDir); err != nil {
			setError("restic restore failed: " + err.Error())
			return
		}

		if isCancelled() {
			setStatus("cancelled", "job_progress_cancelled", 0)
			return
		}

		if tuple.req.ApplyDB {
			setStatus("running", "job_progress_restore_restoring_database", 55)
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

		if isCancelled() {
			setStatus("cancelled", "job_progress_cancelled", 0)
			return
		}

		if tuple.req.ApplyFiles {
			setStatus("running", "job_progress_restore_restoring_files", 75)
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

			totalFiles := 0
			filepath.WalkDir(wpDir, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					totalFiles++
				}
				return nil
			})
			if totalFiles < 1 {
				totalFiles = 1
			}
			fileCount := 0
			rsyncExcludes := []string{"wp-content/cache/"}
			if err := sshClient.SyncDir(wpDir+"/", wpRoot+"/", rsyncExcludes, true, func(relPath string) {
				fileCount++
				pct := 75
				if totalFiles > 0 {
					pct = 75 + int(float64(fileCount)/float64(totalFiles)*25)
				}
				setStatus("running", "Restoring: "+relPath, pct)
			}); err != nil {
				setError("file restore failed: " + err.Error())
				return
			}
		}

		log.Printf("restore completed for site %s: snapshot=%s", site.Name, tuple.req.SnapshotID)
		setResult(fmt.Sprintf(`{"snapshot_id":"%s","apply_db":%t,"apply_files":%t}`, tuple.req.SnapshotID, tuple.req.ApplyDB, tuple.req.ApplyFiles))

	case "upgrade":
		if wpRoot == "" {
			setStatus("running", "job_progress_detecting_wp_root", 10)
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

		setStatus("running", "job_progress_upgrade_running_pipeline", 25)
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

	case "rehearsal":
		if s.StagingManager == nil {
			setError("staging manager not available")
			return
		}

		snapshotID := tuple.req.SnapshotID
		if snapshotID == "" {
			setError("snapshot_id is required")
			return
		}

		setStatus("running", "Creating staging environment...", 10)
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

		resticClient, err := restic.NewClient(resticRepo, resticPassFile)
		if err != nil {
			setError("restic configuration error")
			return
		}

		setStatus("running", "Restoring snapshot to staging...", 30)
		env, err := s.StagingManager.Create(snapshotID, resticClient)
		if err != nil {
			setError("failed to create staging environment: " + err.Error())
			return
		}

		setStatus("running", "Staging environment ready", 90)
		resultJSON, _ := json.Marshal(map[string]interface{}{
			"health_url":   env.HealthURL,
			"compose_file": env.ComposeFile,
			"project_name": env.ProjectName,
			"restore_dir":  env.RestoreDir,
			"wp_version":   env.WPVersion,
			"db_name":      env.DBName,
			"snapshot_id":  snapshotID,
		})
		setResult(string(resultJSON))

	default:
		setError("unknown job type: " + job.Type)
	}
}

func (s *APIServer) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	siteID := r.URL.Query().Get("site_id")
	jobType := r.URL.Query().Get("type")
	all := r.URL.Query().Get("all")

	if siteID != "" {
		if all == "1" || all == "true" {
			jobs, err := s.Database.ListJobsBySiteWithType(siteID, jobType)
			if err != nil {
				apiErr(w, http.StatusInternalServerError, "failed to list jobs")
				return
			}
			jsonResp(w, http.StatusOK, jobs)
			return
		}
		job, err := s.Database.GetLatestJobBySiteWithType(siteID, jobType)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to get job")
			return
		}
		if job == nil {
			jsonResp(w, http.StatusOK, nil)
			return
		}
		jsonResp(w, http.StatusOK, job)
		return
	}

	if all == "1" || all == "true" {
		jobs, err := s.Database.ListJobs()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to list jobs")
			return
		}
		jsonResp(w, http.StatusOK, jobs)
		return
	}

	job, err := s.Database.GetLatestJobByType(jobType)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to get latest job")
		return
	}
	if job == nil {
		jsonResp(w, http.StatusOK, nil)
		return
	}
	jsonResp(w, http.StatusOK, job)
}

func (s *APIServer) handleJobByID(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimPrefix(r.URL.Path, "/api/v1/jobs/")
	if r.Method == "GET" {
		job, err := s.Database.GetJob(jobID)
		if err != nil {
			apiErr(w, http.StatusNotFound, "job not found")
			return
		}
		jsonResp(w, http.StatusOK, job)
		return
	}
	if r.Method == "DELETE" {
		if err := s.Database.DeleteJob(jobID); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to delete job")
			return
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"deleted": true})
		return
	}
	if r.Method == "POST" {
		if err := s.Database.CancelJob(jobID); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to cancel job")
			return
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"cancelled": true})
		return
	}
	apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
}
