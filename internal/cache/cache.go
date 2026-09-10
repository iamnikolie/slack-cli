package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iamnikolie/slack-cli/internal/config"
)

type Channel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"is_private"`
	IsMember  bool   `json:"is_member"`
	IsIM      bool   `json:"is_im"`
	IsMPIM    bool   `json:"is_mpim"`
	User      string `json:"user,omitempty"` // DM partner (IM channels only)
}

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name"`
}

type Directory struct {
	Channels []Channel `json:"channels"`
	Users    []User    `json:"users"`
	SyncedAt time.Time `json:"synced_at"`
}

func path(profile string) (string, error) {
	dir, err := config.Dir(profile)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "directory.json"), nil
}

// IsNotExist reports whether err is a missing-cache error.
func IsNotExist(err error) bool { return os.IsNotExist(err) }

// Load reads the cached directory for a profile.
func Load(profile string) (*Directory, error) {
	p, err := path(profile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err // callers use IsNotExist to detect a cold cache
	}
	var d Directory
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("cache.Load: %w", err)
	}
	return &d, nil
}

// Save writes the directory (stamping SyncedAt) to the profile dir.
func Save(d *Directory, profile string) error {
	p, err := path(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("cache.Save: mkdir: %w", err)
	}
	d.SyncedAt = time.Now().UTC()
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("cache.Save: marshal: %w", err)
	}
	return os.WriteFile(p, data, 0600)
}
