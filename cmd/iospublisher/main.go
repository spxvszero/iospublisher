package main

import (
	"log"
	"net/http"
	"os"

	"iospublisher/internal/auth"
	"iospublisher/internal/runtimeconfig"
	"iospublisher/internal/server"
	"iospublisher/web"
)

func main() {
	logger := log.New(os.Stdout, "iospublisher: ", log.LstdFlags)

	configPath, err := runtimeConfigPath()
	if err != nil {
		logger.Fatal(err)
	}
	cfg, err := runtimeconfig.LoadOrCreate(configPath)
	if err != nil {
		logger.Fatal(err)
	}
	cfg = runtimeconfig.ApplyEnv(cfg, os.LookupEnv)
	if err := cfg.Validate(); err != nil {
		logger.Fatal(err)
	}

	addr := cfg.Addr()
	if value := os.Getenv("IOSPUB_ADDR"); value != "" {
		addr = value
	}
	dataDir := cfg.ResolveDataDir(configPath)
	creds := auth.Credentials{
		User:     cfg.Auth.User,
		Password: cfg.Auth.Password,
	}

	if creds.IsDefault() {
		logger.Println("warning: using default Basic Auth credentials admin/admin")
	}
	logger.Printf("using runtime config %s", configPath)
	logger.Printf("listening on %s, data dir %s, max upload bytes %d", addr, dataDir, cfg.MaxUploadBytes)

	app := server.New(server.Options{
		DataDir:        dataDir,
		Credentials:    creds,
		MaxUploadBytes: cfg.MaxUploadBytes,
		Assets:         web.FS,
		Logger:         logger,
	})
	if err := http.ListenAndServe(addr, app.Handler()); err != nil {
		logger.Fatal(err)
	}
}

func runtimeConfigPath() (string, error) {
	if value := os.Getenv("IOSPUB_CONFIG_PATH"); value != "" {
		return value, nil
	}
	return runtimeconfig.DefaultPath()
}
