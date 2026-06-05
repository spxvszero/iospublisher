package runtimeconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateCreatesDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d", cfg.Port)
	}
	if cfg.Auth.User != "admin" {
		t.Fatalf("Auth.User = %q", cfg.Auth.User)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var written Config
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("created config is not json: %v", err)
	}
	if written.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Fatalf("MaxUploadBytes = %d", written.MaxUploadBytes)
	}
}

func TestLoadOrCreateLoadsCustomConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
		"ip": "127.0.0.1",
		"port": 18080,
		"dataDir": "runtime-data",
		"auth": {
			"user": "ops",
			"password": "secret"
		},
		"maxUploadBytes": 1024
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}
	if cfg.Addr() != "127.0.0.1:18080" {
		t.Fatalf("Addr() = %q", cfg.Addr())
	}
	if got := cfg.ResolveDataDir(path); got != filepath.Join(filepath.Dir(path), "runtime-data") {
		t.Fatalf("ResolveDataDir() = %q", got)
	}
	if cfg.Auth.Password != "secret" {
		t.Fatalf("Auth.Password = %q", cfg.Auth.Password)
	}
	if cfg.MaxUploadBytes != 1024 {
		t.Fatalf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
}

func TestApplyEnvOverridesConfig(t *testing.T) {
	cfg := Default()
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"IOSPUB_IP":               "127.0.0.1",
			"IOSPUB_PORT":             "19090",
			"IOSPUB_DATA_DIR":         "override-data",
			"IOSPUB_ADMIN_USER":       "root",
			"IOSPUB_ADMIN_PASSWORD":   "pass",
			"IOSPUB_MAX_UPLOAD_BYTES": "4096",
		}
		value, ok := values[key]
		return value, ok
	}

	cfg = ApplyEnv(cfg, lookup)
	if cfg.Addr() != "127.0.0.1:19090" {
		t.Fatalf("Addr() = %q", cfg.Addr())
	}
	if cfg.DataDir != "override-data" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.Auth.User != "root" || cfg.Auth.Password != "pass" {
		t.Fatalf("Auth = %#v", cfg.Auth)
	}
	if cfg.MaxUploadBytes != 4096 {
		t.Fatalf("MaxUploadBytes = %d", cfg.MaxUploadBytes)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cfg := Default()
	cfg.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for bad port")
	}

	cfg = Default()
	cfg.MaxUploadBytes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for bad max upload bytes")
	}
}
