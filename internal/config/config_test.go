package config

import (
	"path/filepath"
	"testing"
)

func TestStoreSaveLoad(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))

	saved, err := store.Save(Config{
		AppName:      " Demo App ",
		ReleaseNotes: " 发布说明 \n ",
		IPAURL:       " https://example.com/app.ipa ",
		PlistURL:     " https://example.com/manifest.plist ",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.AppName != "Demo App" {
		t.Fatalf("AppName = %q", saved.AppName)
	}
	if saved.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.AppName != "Demo App" {
		t.Fatalf("loaded AppName = %q", loaded.AppName)
	}
	if loaded.IPAURL != "https://example.com/app.ipa" {
		t.Fatalf("loaded IPAURL = %q", loaded.IPAURL)
	}
	if loaded.ReleaseNotes != "发布说明" {
		t.Fatalf("loaded ReleaseNotes = %q", loaded.ReleaseNotes)
	}
}

func TestStoreLoadDefault(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing.json"))
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppName == "" {
		t.Fatal("default AppName should be set")
	}
}
