package config

import (
	"os"
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

func TestStoreMigratesLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{
		"appName": "Legacy App",
		"releaseNotes": "legacy notes",
		"ipaUrl": "https://example.com/app.ipa",
		"plistUrl": "https://example.com/manifest.plist"
	}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	doc, err := NewStore(path).LoadDocument()
	if err != nil {
		t.Fatalf("LoadDocument() error = %v", err)
	}
	if doc.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %d", doc.SchemaVersion)
	}
	if len(doc.Tags) != 1 || doc.Tags[0].Name != DefaultTagName {
		t.Fatalf("Tags = %#v", doc.Tags)
	}
	if doc.Tags[0].Config.AppName != "Legacy App" {
		t.Fatalf("AppName = %q", doc.Tags[0].Config.AppName)
	}
}

func TestStoreCreateAndDeleteTag(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.json"))

	tag, err := store.CreateTag("Beta_1")
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if tag.Name != "beta_1" {
		t.Fatalf("tag.Name = %q", tag.Name)
	}
	if len(tag.FileKey) != 8 {
		t.Fatalf("FileKey = %q", tag.FileKey)
	}
	if tag.Analysis.Status != AnalysisPending {
		t.Fatalf("Analysis.Status = %q", tag.Analysis.Status)
	}

	if _, err := store.CreateTag("beta_1"); err == nil {
		t.Fatal("CreateTag() expected duplicate error")
	}
	if _, err := store.DeleteTag(DefaultTagName); err == nil {
		t.Fatal("DeleteTag(default) expected error")
	}
	deleted, err := store.DeleteTag("beta_1")
	if err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
	if deleted.FileKey != tag.FileKey {
		t.Fatalf("deleted FileKey = %q, want %q", deleted.FileKey, tag.FileKey)
	}
}
