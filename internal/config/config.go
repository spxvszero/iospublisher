package config

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTagName = "default"
	SchemaVersion  = 2

	AnalysisPending = "pending"
	AnalysisSuccess = "success"
	AnalysisFailed  = "failed"

	PackageDevelopment = "development"
	PackageAdHoc       = "ad-hoc"
	PackageEnterprise  = "enterprise"
	PackageAppStore    = "app-store"
	PackageUnknown     = "unknown"
)

var tagNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type Config struct {
	AppName      string    `json:"appName"`
	ReleaseNotes string    `json:"releaseNotes"`
	IPAURL       string    `json:"ipaUrl"`
	PlistURL     string    `json:"plistUrl"`
	UpdatedAt    time.Time `json:"updatedAt"`
	PublishedAt  time.Time `json:"publishedAt"`
}

type Analysis struct {
	Status               string    `json:"status"`
	PackageType          string    `json:"packageType"`
	DeviceUUIDs          []string  `json:"deviceUUIDs"`
	CertificateExpiresAt time.Time `json:"certificateExpiresAt"`
	ProfileExpiresAt     time.Time `json:"profileExpiresAt"`
	AnalyzedAt           time.Time `json:"analyzedAt"`
	Error                string    `json:"error"`
}

type Tag struct {
	Name     string   `json:"name"`
	FileKey  string   `json:"fileKey"`
	Config   Config   `json:"config"`
	Analysis Analysis `json:"analysis"`
}

type Document struct {
	SchemaVersion int    `json:"schemaVersion"`
	ActiveTag     string `json:"activeTag"`
	Tags          []Tag  `json:"tags"`
}

type Store struct {
	path string
	mu   sync.RWMutex
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (Config, error) {
	doc, err := s.LoadDocument()
	if err != nil {
		return Config{}, err
	}
	tag, ok := doc.FindTag(DefaultTagName)
	if !ok {
		return defaultConfig(), nil
	}
	return tag.Config, nil
}

func (s *Store) Save(cfg Config) (Config, error) {
	tag, err := s.SaveTagConfig(DefaultTagName, cfg)
	if err != nil {
		return Config{}, err
	}
	return tag.Config, nil
}

func (s *Store) LoadDocument() (Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadDocumentLocked()
}

func (s *Store) SaveTagConfig(name string, cfg Config) (Tag, error) {
	return s.updateTag(name, func(tag *Tag) error {
		publishedAt := tag.Config.PublishedAt
		next, err := sanitizeConfig(cfg)
		if err != nil {
			return err
		}
		next.PublishedAt = publishedAt
		next.UpdatedAt = time.Now().UTC()
		tag.Config = next
		return nil
	})
}

func (s *Store) UpdateTag(name string, update func(*Tag) error) (Tag, error) {
	return s.updateTag(name, update)
}

func (s *Store) CreateTag(name string) (Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tagName, err := NormalizeTagName(name)
	if err != nil {
		return Tag{}, err
	}
	if tagName == DefaultTagName {
		return Tag{}, errors.New("default tag already exists")
	}

	doc, err := s.loadDocumentLocked()
	if err != nil {
		return Tag{}, err
	}
	if _, ok := doc.FindTag(tagName); ok {
		return Tag{}, fmt.Errorf("tag %q already exists", tagName)
	}

	fileKey, err := uniqueFileKey(doc)
	if err != nil {
		return Tag{}, err
	}
	defaultTag, _ := doc.FindTag(DefaultTagName)
	tag := Tag{
		Name:    tagName,
		FileKey: fileKey,
		Config: Config{
			AppName: defaultTag.Config.AppName,
		},
		Analysis: defaultAnalysis(),
	}
	if strings.TrimSpace(tag.Config.AppName) == "" {
		tag.Config.AppName = "iOS App"
	}
	doc.Tags = append(doc.Tags, tag)
	doc.ActiveTag = tagName
	if err := s.writeDocumentLocked(doc); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

func (s *Store) DeleteTag(name string) (Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tagName, err := NormalizeTagName(name)
	if err != nil {
		return Tag{}, err
	}
	if tagName == DefaultTagName {
		return Tag{}, errors.New("default tag cannot be deleted")
	}

	doc, err := s.loadDocumentLocked()
	if err != nil {
		return Tag{}, err
	}
	index := slices.IndexFunc(doc.Tags, func(tag Tag) bool {
		return tag.Name == tagName
	})
	if index < 0 {
		return Tag{}, fmt.Errorf("tag %q not found", tagName)
	}
	deleted := doc.Tags[index]
	doc.Tags = append(doc.Tags[:index], doc.Tags[index+1:]...)
	if doc.ActiveTag == tagName {
		doc.ActiveTag = DefaultTagName
	}
	if err := s.writeDocumentLocked(doc); err != nil {
		return Tag{}, err
	}
	return deleted, nil
}

func (s *Store) updateTag(name string, update func(*Tag) error) (Tag, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tagName, err := NormalizeTagName(name)
	if err != nil {
		return Tag{}, err
	}
	doc, err := s.loadDocumentLocked()
	if err != nil {
		return Tag{}, err
	}
	index := slices.IndexFunc(doc.Tags, func(tag Tag) bool {
		return tag.Name == tagName
	})
	if index < 0 {
		return Tag{}, fmt.Errorf("tag %q not found", tagName)
	}
	if err := update(&doc.Tags[index]); err != nil {
		return Tag{}, err
	}
	doc.Tags[index] = normalizeTag(doc.Tags[index])
	if err := s.writeDocumentLocked(doc); err != nil {
		return Tag{}, err
	}
	return doc.Tags[index], nil
}

func (s *Store) loadDocumentLocked() (Document, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultDocument(), nil
		}
		return Document{}, err
	}

	var doc Document
	if err := json.Unmarshal(data, &doc); err == nil && (doc.SchemaVersion == SchemaVersion || len(doc.Tags) > 0) {
		return normalizeDocument(doc), nil
	}

	var legacy Config
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Document{}, err
	}
	legacy = normalizeConfig(legacy)
	return normalizeDocument(Document{
		SchemaVersion: SchemaVersion,
		ActiveTag:     DefaultTagName,
		Tags: []Tag{{
			Name:     DefaultTagName,
			Config:   legacy,
			Analysis: defaultAnalysis(),
		}},
	}), nil
}

func (s *Store) writeDocumentLocked(doc Document) error {
	doc = normalizeDocument(doc)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o644)
}

func (d Document) FindTag(name string) (Tag, bool) {
	tagName, err := NormalizeTagName(name)
	if err != nil {
		return Tag{}, false
	}
	for _, tag := range d.Tags {
		if tag.Name == tagName {
			return tag, true
		}
	}
	return Tag{}, false
}

func NormalizeTagName(name string) (string, error) {
	tagName := strings.ToLower(strings.TrimSpace(name))
	if tagName == "" {
		tagName = DefaultTagName
	}
	if !tagNamePattern.MatchString(tagName) {
		return "", errors.New("tag must use letters, numbers, hyphen or underscore")
	}
	return tagName, nil
}

func sanitizeConfig(cfg Config) (Config, error) {
	cfg = normalizeConfig(cfg)
	if cfg.AppName == "" {
		return Config{}, errors.New("app name is required")
	}
	return cfg, nil
}

func normalizeDocument(doc Document) Document {
	doc.SchemaVersion = SchemaVersion
	if strings.TrimSpace(doc.ActiveTag) == "" {
		doc.ActiveTag = DefaultTagName
	}

	seen := map[string]bool{}
	tags := make([]Tag, 0, len(doc.Tags)+1)
	for _, tag := range doc.Tags {
		tag = normalizeTag(tag)
		if tag.Name == "" || seen[tag.Name] {
			continue
		}
		seen[tag.Name] = true
		tags = append(tags, tag)
	}
	if !seen[DefaultTagName] {
		tags = append([]Tag{{
			Name:     DefaultTagName,
			Config:   defaultConfig(),
			Analysis: defaultAnalysis(),
		}}, tags...)
	} else {
		index := slices.IndexFunc(tags, func(tag Tag) bool {
			return tag.Name == DefaultTagName
		})
		if index > 0 {
			defaultTag := tags[index]
			tags = append(tags[:index], tags[index+1:]...)
			tags = append([]Tag{defaultTag}, tags...)
		}
	}
	doc.Tags = tags
	if _, ok := doc.FindTag(doc.ActiveTag); !ok {
		doc.ActiveTag = DefaultTagName
	}
	return doc
}

func normalizeTag(tag Tag) Tag {
	tag.Name, _ = NormalizeTagName(tag.Name)
	if tag.Name == DefaultTagName {
		tag.FileKey = ""
	} else {
		tag.FileKey = strings.TrimSpace(tag.FileKey)
	}
	tag.Config = normalizeConfig(tag.Config)
	tag.Analysis = normalizeAnalysis(tag.Analysis)
	return tag
}

func normalizeConfig(cfg Config) Config {
	cfg.AppName = strings.TrimSpace(cfg.AppName)
	cfg.ReleaseNotes = strings.TrimSpace(cfg.ReleaseNotes)
	cfg.IPAURL = strings.TrimSpace(cfg.IPAURL)
	cfg.PlistURL = strings.TrimSpace(cfg.PlistURL)
	if cfg.AppName == "" {
		cfg.AppName = "iOS App"
	}
	return cfg
}

func normalizeAnalysis(analysis Analysis) Analysis {
	analysis.Status = strings.TrimSpace(analysis.Status)
	if analysis.Status == "" {
		analysis.Status = AnalysisPending
	}
	analysis.PackageType = strings.TrimSpace(analysis.PackageType)
	if analysis.PackageType == "" {
		analysis.PackageType = PackageUnknown
	}
	analysis.Error = strings.TrimSpace(analysis.Error)
	if analysis.DeviceUUIDs == nil {
		analysis.DeviceUUIDs = []string{}
	}
	return analysis
}

func defaultDocument() Document {
	return Document{
		SchemaVersion: SchemaVersion,
		ActiveTag:     DefaultTagName,
		Tags: []Tag{{
			Name:     DefaultTagName,
			Config:   defaultConfig(),
			Analysis: defaultAnalysis(),
		}},
	}
}

func defaultConfig() Config {
	return Config{AppName: "iOS App"}
}

func defaultAnalysis() Analysis {
	return Analysis{
		Status:      AnalysisPending,
		PackageType: PackageUnknown,
		DeviceUUIDs: []string{},
	}
}

func uniqueFileKey(doc Document) (string, error) {
	used := map[string]bool{}
	for _, tag := range doc.Tags {
		if tag.FileKey != "" {
			used[tag.FileKey] = true
		}
	}
	for i := 0; i < 16; i++ {
		key, err := randomFileKey()
		if err != nil {
			return "", err
		}
		if !used[key] {
			return key, nil
		}
	}
	return "", errors.New("could not generate unique file key")
}

func randomFileKey() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < 8; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String(), nil
}
