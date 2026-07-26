package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

const encryptedV1Prefix = "enc:v1:"

func encryptionKey() []byte {
	key := os.Getenv("DATA_ENCRYPTION_KEY")
	if key == "" {
		key = os.Getenv("SECRET_KEY")
	}
	if key == "" {
		key = "default-secret-key"
	}
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

func encryptSensitive(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, encryptedV1Prefix) {
		return plain, nil
	}

	block, err := aes.NewCipher(encryptionKey())
	if err != nil {
		return "", fmt.Errorf("failed to initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to initialize gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedV1Prefix + base64.StdEncoding.EncodeToString(payload), nil
}

func decryptSensitive(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedV1Prefix) {
		return value, nil
	}

	encoded := strings.TrimPrefix(value, encryptedV1Prefix)
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted value: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey())
	if err != nil {
		return "", fmt.Errorf("failed to initialize cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to initialize gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return "", fmt.Errorf("encrypted payload too short")
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value: %w", err)
	}

	return string(plain), nil
}

func encryptSiteSensitiveFields(site *Site) error {
	var err error
	site.DBPassword, err = encryptSensitive(site.DBPassword)
	if err != nil {
		return fmt.Errorf("failed to encrypt db_password: %w", err)
	}
	site.ResticPasswordFile, err = encryptSensitive(site.ResticPasswordFile)
	if err != nil {
		return fmt.Errorf("failed to encrypt restic_password_file: %w", err)
	}
	return nil
}

func decryptSiteSensitiveFields(site *Site) error {
	var err error
	site.DBPassword, err = decryptSensitive(site.DBPassword)
	if err != nil {
		return fmt.Errorf("failed to decrypt db_password: %w", err)
	}
	site.ResticPasswordFile, err = decryptSensitive(site.ResticPasswordFile)
	if err != nil {
		return fmt.Errorf("failed to decrypt restic_password_file: %w", err)
	}
	return nil
}

func (db *Database) migrateLegacyPlaintextSiteSecrets() error {
	rows, err := db.Query("SELECT id, db_password, restic_password_file FROM sites")
	if err != nil {
		return err
	}
	defer rows.Close()

	type candidate struct {
		id                 string
		dbPassword         sql.NullString
		resticPasswordFile sql.NullString
	}

	var candidates []candidate

	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.dbPassword, &c.resticPasswordFile); err != nil {
			return err
		}
		candidates = append(candidates, c)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range candidates {

		newDBPassword := ""
		newResticPasswordFile := ""
		updateDBPassword := false
		updateResticPasswordFile := false

		if c.dbPassword.Valid && c.dbPassword.String != "" && !strings.HasPrefix(c.dbPassword.String, encryptedV1Prefix) {
			encValue, err := encryptSensitive(c.dbPassword.String)
			if err != nil {
				return err
			}
			newDBPassword = encValue
			updateDBPassword = true
		}

		if c.resticPasswordFile.Valid && c.resticPasswordFile.String != "" && !strings.HasPrefix(c.resticPasswordFile.String, encryptedV1Prefix) {
			encValue, err := encryptSensitive(c.resticPasswordFile.String)
			if err != nil {
				return err
			}
			newResticPasswordFile = encValue
			updateResticPasswordFile = true
		}

		switch {
		case updateDBPassword && updateResticPasswordFile:
			if _, err := db.Exec("UPDATE sites SET db_password = ?, restic_password_file = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newDBPassword, newResticPasswordFile, c.id); err != nil {
				return err
			}
		case updateDBPassword:
			if _, err := db.Exec("UPDATE sites SET db_password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newDBPassword, c.id); err != nil {
				return err
			}
		case updateResticPasswordFile:
			if _, err := db.Exec("UPDATE sites SET restic_password_file = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newResticPasswordFile, c.id); err != nil {
				return err
			}
		}
	}

	return nil
}
