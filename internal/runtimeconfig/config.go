package runtimeconfig

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultMaxUploadBytes int64 = 2 * 1024 * 1024 * 1024

type Config struct {
	IP             string     `json:"ip"`
	Port           int        `json:"port"`
	DataDir        string     `json:"dataDir"`
	Auth           AuthConfig `json:"auth"`
	MaxUploadBytes int64      `json:"maxUploadBytes"`
}

type AuthConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

func Default() Config {
	return Config{
		IP:      "0.0.0.0",
		Port:    8080,
		DataDir: "data",
		Auth: AuthConfig{
			User:     "admin",
			Password: "admin",
		},
		MaxUploadBytes: DefaultMaxUploadBytes,
	}
}

func DefaultPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "config.json"), nil
}

func LoadOrCreate(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := Default()
			if err := Write(path, cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, err
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Write(path string, cfg Config) error {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (c Config) WithDefaults() Config {
	def := Default()
	c.IP = strings.TrimSpace(c.IP)
	c.DataDir = strings.TrimSpace(c.DataDir)
	c.Auth.User = strings.TrimSpace(c.Auth.User)
	c.Auth.Password = strings.TrimSpace(c.Auth.Password)
	if c.IP == "" {
		c.IP = def.IP
	}
	if c.Port == 0 {
		c.Port = def.Port
	}
	if c.DataDir == "" {
		c.DataDir = def.DataDir
	}
	if c.Auth.User == "" {
		c.Auth.User = def.Auth.User
	}
	if c.Auth.Password == "" {
		c.Auth.Password = def.Auth.Password
	}
	if c.MaxUploadBytes == 0 {
		c.MaxUploadBytes = def.MaxUploadBytes
	}
	return c
}

func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if c.Auth.User == "" {
		return errors.New("auth user is required")
	}
	if c.Auth.Password == "" {
		return errors.New("auth password is required")
	}
	if c.MaxUploadBytes <= 0 {
		return errors.New("max upload bytes must be greater than 0")
	}
	return nil
}

func (c Config) Addr() string {
	return net.JoinHostPort(c.IP, strconv.Itoa(c.Port))
}

func (c Config) ResolveDataDir(configPath string) string {
	if filepath.IsAbs(c.DataDir) {
		return c.DataDir
	}
	return filepath.Join(filepath.Dir(configPath), c.DataDir)
}

func ApplyEnv(c Config, lookup func(string) (string, bool)) Config {
	if value, ok := lookup("IOSPUB_IP"); ok && strings.TrimSpace(value) != "" {
		c.IP = strings.TrimSpace(value)
	}
	if value, ok := lookup("IOSPUB_PORT"); ok && strings.TrimSpace(value) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			c.Port = parsed
		}
	}
	if value, ok := lookup("IOSPUB_DATA_DIR"); ok && strings.TrimSpace(value) != "" {
		c.DataDir = strings.TrimSpace(value)
	}
	if value, ok := lookup("IOSPUB_ADMIN_USER"); ok && strings.TrimSpace(value) != "" {
		c.Auth.User = strings.TrimSpace(value)
	}
	if value, ok := lookup("IOSPUB_ADMIN_PASSWORD"); ok && strings.TrimSpace(value) != "" {
		c.Auth.Password = strings.TrimSpace(value)
	}
	if value, ok := lookup("IOSPUB_MAX_UPLOAD_BYTES"); ok && strings.TrimSpace(value) != "" {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			c.MaxUploadBytes = parsed
		}
	}
	return c.WithDefaults()
}
