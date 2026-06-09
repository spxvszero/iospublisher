package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"iospublisher/internal/auth"
	"iospublisher/internal/config"
	"iospublisher/internal/ipa"
	"iospublisher/internal/plist"
	"iospublisher/internal/qrcode"
)

var errTagNotFound = errors.New("tag not found")

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
	Tag            string          `json:"tag"`
	FileKey        string          `json:"fileKey"`
	IsDefault      bool            `json:"isDefault"`
	Config         config.Config   `json:"config"`
	Analysis       config.Analysis `json:"analysis"`
	HasIPA         bool            `json:"hasIpa"`
	RemoteIPA      bool            `json:"remoteIpa"`
	IPASize        int64           `json:"ipaSize"`
	IPACreatedAt   time.Time       `json:"ipaCreatedAt"`
	HasPlist       bool            `json:"hasPlist"`
	PlistSize      int64           `json:"plistSize"`
	IPAFilename    string          `json:"ipaFilename"`
	PlistFilename  string          `json:"plistFilename"`
	InstallURL     string          `json:"installUrl"`
	QRURL          string          `json:"qrUrl"`
	MaxUploadBytes int64           `json:"maxUploadBytes"`
	Ready          bool            `json:"ready"`
}

type publishResponse struct {
	Config         config.Config   `json:"config"`
	Analysis       config.Analysis `json:"analysis"`
	HasIPA         bool            `json:"hasIpa"`
	IPASize        int64           `json:"ipaSize"`
	HasPlist       bool            `json:"hasPlist"`
	PlistSize      int64           `json:"plistSize"`
	InstallURL     string          `json:"installUrl"`
	QRURL          string          `json:"qrUrl"`
	MaxUploadBytes int64           `json:"maxUploadBytes"`
	Ready          bool            `json:"ready"`
	Tags           []stateResponse `json:"tags"`
}

type tagsResponse struct {
	ActiveTag string          `json:"activeTag"`
	Tags      []stateResponse `json:"tags"`
}

type tagInput struct {
	Name string `json:"name"`
}

type uuidSearchResponse struct {
	Tag     string   `json:"tag"`
	Query   string   `json:"query"`
	Exists  bool     `json:"exists"`
	Matches []string `json:"matches"`
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
	mux.Handle("/api/tags", auth.Middleware(s.credentials, http.HandlerFunc(s.handleTags)))
	mux.Handle("/api/state", auth.Middleware(s.credentials, http.HandlerFunc(s.handleState)))
	mux.Handle("/api/upload", auth.Middleware(s.credentials, http.HandlerFunc(s.handleUpload)))
	mux.Handle("/api/config", auth.Middleware(s.credentials, http.HandlerFunc(s.handleConfig)))
	mux.Handle("/api/plist/generate", auth.Middleware(s.credentials, http.HandlerFunc(s.handleGeneratePlist)))
	mux.HandleFunc("/api/publish", s.handlePublishState)
	mux.HandleFunc("/api/uuid/search", s.handleUUIDSearch)
	mux.HandleFunc("/assets/app.js", s.serveAsset("app.js", "text/javascript; charset=utf-8"))
	mux.HandleFunc("/assets/style.css", s.serveAsset("style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/files/", s.handleFiles)
	mux.HandleFunc("/manifest.plist", s.handleManifest)
	mux.HandleFunc("/qr.png", s.handleQR)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if isTaggedManifestPath(r.URL.Path) {
		s.handleTaggedManifest(w, r)
		return
	}
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
	doc, err := s.store.LoadDocument()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	states, err := s.statesForDocument(r, doc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defaultState := states[0]
	writeJSON(w, http.StatusOK, publishResponse{
		Config:         defaultState.Config,
		Analysis:       defaultState.Analysis,
		HasIPA:         defaultState.HasIPA,
		IPASize:        defaultState.IPASize,
		HasPlist:       defaultState.HasPlist,
		PlistSize:      defaultState.PlistSize,
		InstallURL:     defaultState.InstallURL,
		QRURL:          defaultState.QRURL,
		MaxUploadBytes: defaultState.MaxUploadBytes,
		Ready:          defaultState.Ready,
		Tags:           states,
	})
}

func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if !state.Ready {
		http.Redirect(w, r, "/publish", http.StatusFound)
		return
	}
	http.Redirect(w, r, manifestInstallURL(state.Config.PlistURL), http.StatusFound)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := s.store.LoadDocument()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		states, err := s.statesForDocument(r, doc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, tagsResponse{ActiveTag: doc.ActiveTag, Tags: states})
	case http.MethodPost:
		var input tagInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		tag, err := s.store.CreateTag(input.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		state, err := s.stateForTag(r, tag.Name)
		if err != nil {
			writeError(w, statusForError(err), err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodDelete:
		tagName, err := requestTag(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		deleted, err := s.store.DeleteTag(tagName)
		if err != nil {
			writeError(w, statusForError(err), err)
			return
		}
		if err := s.removeTagFiles(deleted); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		doc, err := s.store.LoadDocument()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		states, err := s.statesForDocument(r, doc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, tagsResponse{ActiveTag: doc.ActiveTag, Tags: states})
	default:
		methodNotAllowed(w, "GET, POST, DELETE")
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	tagName, err := requestTag(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var cfg config.Config
	if err := decodeJSON(w, r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.store.SaveTagConfig(tagName, cfg); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	state, err := s.stateForTag(r, tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleUploadPost(w, r)
	case http.MethodDelete:
		s.handleUploadDelete(w, r)
	default:
		methodNotAllowed(w, "POST, DELETE")
	}
}

func (s *Server) handleUploadPost(w http.ResponseWriter, r *http.Request) {
	tagName, err := requestTag(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tag, err := s.tagByName(tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
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

		dstPath := s.ipaPath(tag)
		tmpPath := dstPath + ".upload"
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

		_ = os.Remove(dstPath)
		if err := os.Rename(tmpPath, dstPath); err != nil {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		analysis := ipa.Analyze(dstPath)
		_, err = s.store.UpdateTag(tagName, func(tag *config.Tag) error {
			tag.Config.PublishedAt = time.Now().UTC()
			tag.Analysis = analysis
			return nil
		})
		if err != nil {
			writeError(w, statusForError(err), err)
			return
		}
		state, err := s.stateForTag(r, tagName)
		if err != nil {
			writeError(w, statusForError(err), err)
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

func (s *Server) handleUploadDelete(w http.ResponseWriter, r *http.Request) {
	tagName, err := requestTag(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tag, err := s.tagByName(tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	if err := os.Remove(s.ipaPath(tag)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	_, err = s.store.UpdateTag(tagName, func(tag *config.Tag) error {
		tag.Config.PublishedAt = time.Time{}
		tag.Analysis = config.Analysis{
			Status:      config.AnalysisPending,
			PackageType: config.PackageUnknown,
			DeviceUUIDs: []string{},
		}
		return nil
	})
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	state, err := s.stateForTag(r, tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handleGeneratePlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	tagName, err := requestTag(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	tag, err := s.tagByName(tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	var input plist.GenerateInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hasCustomIPAURL := strings.TrimSpace(tag.Config.IPAURL) != ""
	tag = s.withDefaultURLs(r, tag)
	if err := s.ensureIPAAvailable(tag, hasCustomIPAURL); err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	data, err := plist.Generate(tag.Config, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.writeFileAtomic(s.plistPath(tag), data); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	state, err := s.stateForTag(r, tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) ensureIPAAvailable(tag config.Tag, allowRemote bool) error {
	if _, err := os.Stat(s.ipaPath(tag)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !allowRemote {
		return errors.New("ipa file is required before generating plist")
	}
	return checkRemoteIPA(tag.Config.IPAURL)
}

func checkRemoteIPA(ipaURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(ipaURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("ipa url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("ipa url must use http or https")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodHead, parsed.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ipa url is not reachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("ipa url head returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	filename := path.Base(r.URL.Path)
	tag, err := s.tagByIPAFilename(filename)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, s.ipaFilename(tag)))
	http.ServeFile(w, r, s.ipaPath(tag))
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	tag, err := s.tagByName(config.DefaultTagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	s.serveManifestForTag(w, r, tag)
}

func (s *Server) handleTaggedManifest(w http.ResponseWriter, r *http.Request) {
	fileKey := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/manifest-"), ".plist")
	tag, err := s.tagByFileKey(fileKey)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	s.serveManifestForTag(w, r, tag)
}

func (s *Server) serveManifestForTag(w http.ResponseWriter, r *http.Request, tag config.Tag) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, s.plistFilename(tag)))
	}
	http.ServeFile(w, r, s.plistPath(tag))
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	state, err := s.state(r)
	if err != nil {
		writeError(w, statusForError(err), err)
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

func (s *Server) handleUUIDSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	tagName, err := requestTag(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(query) < 4 {
		writeError(w, http.StatusBadRequest, errors.New("uuid query must be at least 4 characters"))
		return
	}
	tag, err := s.tagByName(tagName)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	matches := []string{}
	if tag.Analysis.PackageType == config.PackageDevelopment && len(tag.Analysis.DeviceUUIDs) > 0 {
		for _, uuid := range tag.Analysis.DeviceUUIDs {
			if strings.Contains(strings.ToLower(uuid), query) {
				matches = append(matches, uuid)
				if len(matches) == 20 {
					break
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, uuidSearchResponse{
		Tag:     tag.Name,
		Query:   query,
		Exists:  len(matches) > 0,
		Matches: matches,
	})
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
	tagName, err := requestTag(r)
	if err != nil {
		return stateResponse{}, err
	}
	return s.stateForTag(r, tagName)
}

func (s *Server) stateForTag(r *http.Request, tagName string) (stateResponse, error) {
	tag, err := s.tagByName(tagName)
	if err != nil {
		return stateResponse{}, err
	}
	return s.stateForTagObject(r, tag)
}

func (s *Server) statesForDocument(r *http.Request, doc config.Document) ([]stateResponse, error) {
	states := make([]stateResponse, 0, len(doc.Tags))
	for _, tag := range doc.Tags {
		state, err := s.stateForTagObject(r, tag)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return []stateResponse{}, errors.New("default tag is missing")
	}
	return states, nil
}

func (s *Server) stateForTagObject(r *http.Request, tag config.Tag) (stateResponse, error) {
	hasConfiguredIPAURL := strings.TrimSpace(tag.Config.IPAURL) != ""
	tag = s.withDefaultURLs(r, tag)
	ipaInfo, ipaErr := os.Stat(s.ipaPath(tag))
	plistInfo, plistErr := os.Stat(s.plistPath(tag))
	if ipaErr != nil && !errors.Is(ipaErr, os.ErrNotExist) {
		return stateResponse{}, ipaErr
	}
	if plistErr != nil && !errors.Is(plistErr, os.ErrNotExist) {
		return stateResponse{}, plistErr
	}

	hasIPA := ipaErr == nil && !ipaInfo.IsDir()
	hasPlist := plistErr == nil && !plistInfo.IsDir()
	hasRemoteIPA := !hasIPA && hasConfiguredIPAURL
	var ipaSize int64
	var ipaCreatedAt time.Time
	if hasIPA {
		ipaSize = ipaInfo.Size()
		ipaCreatedAt = ipaInfo.ModTime().UTC()
	}
	var plistSize int64
	if hasPlist {
		plistSize = plistInfo.Size()
	}

	return stateResponse{
		Tag:            tag.Name,
		FileKey:        tag.FileKey,
		IsDefault:      tag.Name == config.DefaultTagName,
		Config:         tag.Config,
		Analysis:       tag.Analysis,
		HasIPA:         hasIPA,
		RemoteIPA:      hasRemoteIPA,
		IPASize:        ipaSize,
		IPACreatedAt:   ipaCreatedAt,
		HasPlist:       hasPlist,
		PlistSize:      plistSize,
		IPAFilename:    s.ipaFilename(tag),
		PlistFilename:  s.plistFilename(tag),
		InstallURL:     s.absoluteURL(r, s.installPath(tag)),
		QRURL:          s.qrPath(tag),
		MaxUploadBytes: s.maxUploadBytes,
		Ready:          (hasIPA || hasRemoteIPA) && hasPlist && strings.TrimSpace(tag.Config.AppName) != "" && strings.TrimSpace(tag.Config.PlistURL) != "",
	}, nil
}

func (s *Server) withDefaultURLs(r *http.Request, tag config.Tag) config.Tag {
	if strings.TrimSpace(tag.Config.IPAURL) == "" {
		tag.Config.IPAURL = s.absoluteURL(r, s.ipaURLPath(tag))
	}
	if strings.TrimSpace(tag.Config.PlistURL) == "" {
		tag.Config.PlistURL = s.absoluteURL(r, s.plistURLPath(tag))
	}
	return tag
}

func (s *Server) tagByName(name string) (config.Tag, error) {
	tagName, err := config.NormalizeTagName(name)
	if err != nil {
		return config.Tag{}, err
	}
	doc, err := s.store.LoadDocument()
	if err != nil {
		return config.Tag{}, err
	}
	tag, ok := doc.FindTag(tagName)
	if !ok {
		return config.Tag{}, fmt.Errorf("%w: %s", errTagNotFound, tagName)
	}
	return tag, nil
}

func (s *Server) tagByFileKey(fileKey string) (config.Tag, error) {
	fileKey = strings.TrimSpace(fileKey)
	if fileKey == "" {
		return config.Tag{}, fmt.Errorf("%w: empty file key", errTagNotFound)
	}
	doc, err := s.store.LoadDocument()
	if err != nil {
		return config.Tag{}, err
	}
	for _, tag := range doc.Tags {
		if tag.FileKey == fileKey {
			return tag, nil
		}
	}
	return config.Tag{}, fmt.Errorf("%w: %s", errTagNotFound, fileKey)
}

func (s *Server) tagByIPAFilename(filename string) (config.Tag, error) {
	if filename == "app.ipa" {
		return s.tagByName(config.DefaultTagName)
	}
	if strings.HasPrefix(filename, "app-") && strings.HasSuffix(filename, ".ipa") {
		return s.tagByFileKey(strings.TrimSuffix(strings.TrimPrefix(filename, "app-"), ".ipa"))
	}
	return config.Tag{}, fmt.Errorf("%w: %s", errTagNotFound, filename)
}

func (s *Server) removeTagFiles(tag config.Tag) error {
	for _, target := range []string{s.ipaPath(tag), s.plistPath(tag)} {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
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

func (s *Server) ipaPath(tag config.Tag) string {
	return filepath.Join(s.dataDir, s.ipaFilename(tag))
}

func (s *Server) plistPath(tag config.Tag) string {
	return filepath.Join(s.dataDir, s.plistFilename(tag))
}

func (s *Server) ipaFilename(tag config.Tag) string {
	if tag.Name == config.DefaultTagName {
		return "app.ipa"
	}
	return "app-" + tag.FileKey + ".ipa"
}

func (s *Server) plistFilename(tag config.Tag) string {
	if tag.Name == config.DefaultTagName {
		return "manifest.plist"
	}
	return "manifest-" + tag.FileKey + ".plist"
}

func (s *Server) ipaURLPath(tag config.Tag) string {
	return "/files/" + s.ipaFilename(tag)
}

func (s *Server) plistURLPath(tag config.Tag) string {
	if tag.Name == config.DefaultTagName {
		return "/manifest.plist"
	}
	return "/" + s.plistFilename(tag)
}

func (s *Server) installPath(tag config.Tag) string {
	if tag.Name == config.DefaultTagName {
		return "/install"
	}
	return "/install?tag=" + url.QueryEscape(tag.Name)
}

func (s *Server) qrPath(tag config.Tag) string {
	if tag.Name == config.DefaultTagName {
		return "/qr.png"
	}
	return "/qr.png?tag=" + url.QueryEscape(tag.Name)
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

func requestTag(r *http.Request) (string, error) {
	return config.NormalizeTagName(r.URL.Query().Get("tag"))
}

func isTaggedManifestPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/manifest-") && strings.HasSuffix(requestPath, ".plist")
}

func statusForError(err error) int {
	if errors.Is(err, errTagNotFound) {
		return http.StatusNotFound
	}
	if strings.Contains(err.Error(), "tag must") ||
		strings.Contains(err.Error(), "app name is required") ||
		strings.Contains(err.Error(), "ipa file is required") ||
		strings.Contains(err.Error(), "ipa url") ||
		strings.Contains(err.Error(), "default tag") ||
		strings.Contains(err.Error(), "already exists") {
		return http.StatusBadRequest
	}
	if strings.Contains(err.Error(), "not found") {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
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
