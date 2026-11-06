package apiserver

import (
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
)

const cveCacheTTL = 24 * time.Hour

// dbCVECache persists CVE provider responses so repeated rehearsals do not
// exhaust WPScan's daily request quota.
type dbCVECache struct {
	db *db.Database
}

func (c *dbCVECache) Get(key string) ([]byte, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}
	payload, fetchedAt, err := c.db.GetCVECache(key)
	if err != nil || time.Since(fetchedAt) > cveCacheTTL {
		return nil, false
	}
	return payload, true
}

func (c *dbCVECache) Set(key string, value []byte) error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.SetCVECache(key, value)
}
