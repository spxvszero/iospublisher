package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"iospublisher/internal/auth"
	"iospublisher/internal/config"
	"iospublisher/internal/plist"
	"iospublisher/internal/qrcode"
)

type Options struct {
	DataDir        string
	Credentials    auth.Credentials
	MaxUploadBytes int64
	Assets         fs.FS
	Logger         *log.Logger
}

type Server struct {
	dataDir        string
	store          *config.Store
	credentials    auth.Credentials
	maxUploadBytes int64
	assets         fs.FS
	logger         *log.Logger
}

type stateResponse struct {
	Config         config.Config `json:"config"`
	HasIPA         bool          `json:"hasIpa"`
	IPASize        int64         `json:"ipaSize"`
	HasPlist       bool          `json:"hasPlist"`
	PlistSize      int64         `json:"plistSize"`
	InstallURL     string        `json:"installUrl"`
	QRURL          string        `json:"qrUrl"`
	MaxUploadBytes int64         `json:"maxUploadBytes"`
	Ready          bool          `json:"ready"`
}

func New(opts Options) *Server {
	if opts.DataDir == "" {
		opts.DataDir = "data"
	}
	if opts.MaxUploadBytes <= 0 {
		opts.MaxUploadBytes = 2 * 1024 * 1024 * 1024
	}
	if opts.Logger == nil {
		opts.Logger = log.New(io.Discard, "", 0)
	}
	return &Server{
		dataDir:        opts.DataDir,
		store:          config.NewStore(filepath.Join(opts.DataDir, "config.json")),
		credentials:    opts.Credentials,
		maxUploadBytes: opts.MaxUploadBytes,
		assets:         opts.Assets,
		logger:         opts.Logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/publish", s.handlePublish)
	mux.HandleFunc("/install", s.handleInstall)
	mux.Handle("/internal", auth.Middleware(s.credentials, http.HandlerFunc(s.handleInternal)))
	mux.Handle("/api/state", auth.Middleware(s.credentials, http.HandlerFunc(s.handleState)))
	mux.Handle("/api/upload", auth.Middleware(s.credentials, http.HandlerFunc(s.handleUpload)))
	mux.Handle("/api/config", auth.Middleware(s.credentials, http.HandlerFunc(s.handleConfig)))
	mux.Handle("/api/plist/generate", auth.Middleware(s.credentials, http.HandlerFunc(s.handleGeneratePlist)))
	mux.HandleFunc("/api/publish", s.handlePublishState)
	mux.HandleFunc("/assets/app.js", s.serveAsset("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/assets/style.css", s.serveAsset("style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/files/app.ipa", s.handleIPA)
	mux.HandleFunc("/manifest.plist", s.handleManifest)
	mux.HandleFunc("/qr.png", s.handleQR)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/publish", http.StatusFound)
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.writeAsset(w, "publish.html", "text/html; charset=utf-8")
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.writeAsset(w, "internal.html", "text/html; charset=utf-8")
}

func (s *Server) handlePublishState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !state.Ready {
		http.Redirect(w, r, "/publish", http.StatusFound)
		return
	}
	http.Redirect(w, r, manifestInstallURL(state.Config.PlistURL), http.StatusFound)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var cfg config.Config
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	saved, err := s.store.Save(cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": saved})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("multipart form is required"))
		return
	}

	var uploaded bool
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if part.FormName() != "ipa" {
			continue
		}
		uploaded = true
		if filename := strings.ToLower(part.FileName()); filename != "" && !strings.HasSuffix(filename, ".ipa") {
			writeError(w, http.StatusBadRequest, errors.New("uploaded file must use .ipa extension"))
			return
		}

		tmpPath := filepath.Join(s.dataDir, "app.ipa.upload")
		out, err := os.Create(tmpPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		limited := &io.LimitedReader{R: part, N: s.maxUploadBytes + 1}
		written, copyErr := io.Copy(out, limited)
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, copyErr)
			return
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, closeErr)
			return
		}
		if written > s.maxUploadBytes {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusRequestEntityTooLarge, errors.New("uploaded ipa exceeds max size"))
			return
		}

		dstPath := s.ipaPath()
		_ = os.Remove(dstPath)
		if err := os.Rename(tmpPath, dstPath); err != nil {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		state, err := s.state(r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	}
	if !uploaded {
		writeError(w, http.StatusBadRequest, errors.New("form field ipa is required"))
		return
	}
}

func (s *Server) handleGeneratePlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if _, err := os.Stat(s.ipaPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusBadRequest, errors.New("ipa file is required before generating plist"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var input plist.GenerateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, err := s.store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg = s.withDefaultURLs(r, cfg)
	data, err := plist.Generate(cfg, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.writeFileAtomic(s.plistPath(), data); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleIPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="app.ipa"`)
	http.ServeFile(w, r, s.ipaPath())
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="manifest.plist"`)
	}
	http.ServeFile(w, r, s.plistPath())
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !state.Ready {
		http.NotFound(w, r)
		return
	}
	pngData, err := qrcode.PNG(state.InstallURL, 6, 4)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(pngData)
}

func (s *Server) serveAsset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		s.writeAsset(w, name, contentType)
	}
}

func (s *Server) writeAsset(w http.ResponseWriter, name, contentType string) {
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (s *Server) state(r *http.Request) (stateResponse, error) {
	cfg, err := s.store.Load()
	if err != nil {
		return stateResponse{}, err
	}
	cfg = s.withDefaultURLs(r, cfg)

	ipaInfo, ipaErr := os.Stat(s.ipaPath())
	plistInfo, plistErr := os.Stat(s.plistPath())

	hasIPA := ipaErr == nil && !ipaInfo.IsDir()
	hasPlist := plistErr == nil && !plistInfo.IsDir()
	var ipaSize int64
	if hasIPA {
		ipaSize = ipaInfo.Size()
	}
	var plistSize int64
	if hasPlist {
		plistSize = plistInfo.Size()
	}

	installURL := s.absoluteURL(r, "/install")
	return stateResponse{
		Config:         cfg,
		HasIPA:         hasIPA,
		IPASize:        ipaSize,
		HasPlist:       hasPlist,
		PlistSize:      plistSize,
		InstallURL:     installURL,
		QRURL:          "/qr.png",
		MaxUploadBytes: s.maxUploadBytes,
		Ready:          hasIPA && hasPlist && strings.TrimSpace(cfg.AppName) != "" && strings.TrimSpace(cfg.PlistURL) != "",
	}, nil
}

func (s *Server) withDefaultURLs(r *http.Request, cfg config.Config) config.Config {
	if strings.TrimSpace(cfg.IPAURL) == "" {
		cfg.IPAURL = s.absoluteURL(r, "/files/app.ipa")
	}
	if strings.TrimSpace(cfg.PlistURL) == "" {
		cfg.PlistURL = s.absoluteURL(r, "/manifest.plist")
	}
	return cfg
}

func (s *Server) absoluteURL(r *http.Request, path string) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost"
	}
	if strings.Contains(host, ",") {
		host = strings.TrimSpace(strings.Split(host, ",")[0])
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		if (scheme == "http" && p == "80") || (scheme == "https" && p == "443") {
			host = h
		}
	}
	return scheme + "://" + host + path
}

func (s *Server) ipaPath() string {
	return filepath.Join(s.dataDir, "app.ipa")
}

func (s *Server) plistPath() string {
	return filepath.Join(s.dataDir, "manifest.plist")
}

func (s *Server) writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func manifestInstallURL(plistURL string) string {
	return "itms-services://?action=download-manifest&url=" + url.QueryEscape(plistURL)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func ParseMaxUploadBytes(value string, fallback int64) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
