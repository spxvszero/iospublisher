package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	AppName      string    `json:"appName"`
	ReleaseNotes string    `json:"releaseNotes"`
	IPAURL       string    `json:"ipaUrl"`
	PlistURL     string    `json:"plistUrl"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Store struct {
	path string
	mu   sync.RWMutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{AppName: "iOS App"}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = "iOS App"
	}
	return cfg, nil
}

func (s *Store) Save(cfg Config) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg.AppName = strings.TrimSpace(cfg.AppName)
	cfg.ReleaseNotes = strings.TrimSpace(cfg.ReleaseNotes)
	cfg.IPAURL = strings.TrimSpace(cfg.IPAURL)
	cfg.PlistURL = strings.TrimSpace(cfg.PlistURL)
	if cfg.AppName == "" {
		return Config{}, errors.New("app name is required")
	}
	cfg.UpdatedAt = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return Config{}, err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
