package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"iospublisher/internal/auth"
	"iospublisher/internal/config"
)

func TestInternalRouteRequiresAuth(t *testing.T) {
	app := testServer(t)

	unauthorized := httptest.NewRecorder()
	app.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/internal", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	req.SetBasicAuth("admin", "secret")
	app.Handler().ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestPublishFlow(t *testing.T) {
	app := testServer(t)
	handler := app.Handler()

	doAuthed(t, handler, http.MethodPost, "/api/config", strings.NewReader(`{
		"appName":"Demo App",
		"releaseNotes":"修复登录问题\n优化安装流程",
		"ipaUrl":"https://example.com/files/app.ipa",
		"plistUrl":"https://example.com/manifest.plist"
	}`), "application/json")

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("ipa", "demo.ipa")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("fake ipa")); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	uploadResponse := doAuthed(t, handler, http.MethodPost, "/api/upload", &upload, writer.FormDataContentType())
	var uploadState stateResponse
	decodeResponse(t, uploadResponse, &uploadState)
	if uploadState.Config.PublishedAt.IsZero() {
		t.Fatal("PublishedAt should be set after upload")
	}
	if uploadState.IPACreatedAt.IsZero() {
		t.Fatal("IPACreatedAt should be set after upload")
	}
	if uploadState.Analysis.Status != config.AnalysisFailed {
		t.Fatalf("fake IPA analysis status = %q", uploadState.Analysis.Status)
	}

	doAuthed(t, handler, http.MethodPost, "/api/plist/generate", strings.NewReader(`{
		"bundleIdentifier":"com.example.demo",
		"bundleVersion":"1.0.0",
		"title":"Demo Install Title"
	}`), "application/json")

	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/manifest.plist", nil))
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest status = %d", manifest.Code)
	}
	if !strings.Contains(manifest.Body.String(), "com.example.demo") {
		t.Fatalf("manifest body missing bundle identifier: %s", manifest.Body.String())
	}
	if !strings.Contains(manifest.Body.String(), "Demo Install Title") {
		t.Fatalf("manifest body missing title: %s", manifest.Body.String())
	}

	download := httptest.NewRecorder()
	handler.ServeHTTP(download, httptest.NewRequest(http.MethodGet, "/manifest.plist?download=1", nil))
	if download.Code != http.StatusOK {
		t.Fatalf("manifest download status = %d", download.Code)
	}
	if got := download.Header().Get("Content-Disposition"); got != `attachment; filename="manifest.plist"` {
		t.Fatalf("manifest download disposition = %q", got)
	}

	install := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/install", nil)
	req.Host = "publisher.example.com"
	handler.ServeHTTP(install, req)
	if install.Code != http.StatusFound {
		t.Fatalf("install status = %d", install.Code)
	}
	if got := install.Header().Get("Location"); !strings.HasPrefix(got, "itms-services://?action=download-manifest") {
		t.Fatalf("install location = %q", got)
	}

	qr := httptest.NewRecorder()
	handler.ServeHTTP(qr, httptest.NewRequest(http.MethodGet, "/qr.png", nil))
	if qr.Code != http.StatusOK {
		t.Fatalf("qr status = %d, body = %s", qr.Code, qr.Body.String())
	}
	if got := qr.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("qr content type = %q", got)
	}
}

func TestDeleteUploadedIPA(t *testing.T) {
	app := testServer(t)
	handler := app.Handler()
	uploadFakeIPA(t, handler, "/api/upload")

	deleteResponse := doAuthed(t, handler, http.MethodDelete, "/api/upload", nil, "")
	var state stateResponse
	decodeResponse(t, deleteResponse, &state)
	if state.HasIPA {
		t.Fatalf("HasIPA = true after delete: %#v", state)
	}
	if !state.Config.PublishedAt.IsZero() {
		t.Fatalf("PublishedAt = %v, want zero", state.Config.PublishedAt)
	}
	if state.Analysis.Status != config.AnalysisPending {
		t.Fatalf("Analysis.Status = %q, want %q", state.Analysis.Status, config.AnalysisPending)
	}
	if _, err := os.Stat(filepath.Join(app.dataDir, "app.ipa")); !os.IsNotExist(err) {
		t.Fatalf("ipa should be deleted, stat err = %v", err)
	}

	secondDelete := doAuthed(t, handler, http.MethodDelete, "/api/upload", nil, "")
	decodeResponse(t, secondDelete, &state)
	if state.HasIPA {
		t.Fatalf("HasIPA = true after second delete: %#v", state)
	}
}

func TestMultiTagPublishFlow(t *testing.T) {
	app := testServer(t)
	handler := app.Handler()

	create := doAuthed(t, handler, http.MethodPost, "/api/tags", strings.NewReader(`{"name":"Beta_1"}`), "application/json")
	var beta stateResponse
	decodeResponse(t, create, &beta)
	if beta.Tag != "beta_1" {
		t.Fatalf("created tag = %q", beta.Tag)
	}
	if len(beta.FileKey) != 8 {
		t.Fatalf("FileKey = %q", beta.FileKey)
	}

	doAuthed(t, handler, http.MethodPost, "/api/config?tag=beta_1", strings.NewReader(`{
		"appName":"Beta App",
		"releaseNotes":"beta notes",
		"ipaUrl":"",
		"plistUrl":""
	}`), "application/json")
	uploadFakeIPA(t, handler, "/api/upload?tag=beta_1")
	doAuthed(t, handler, http.MethodPost, "/api/plist/generate?tag=beta_1", strings.NewReader(`{
		"bundleIdentifier":"com.example.beta",
		"bundleVersion":"2.0.0",
		"title":"Beta Install"
	}`), "application/json")

	ipaFile := httptest.NewRecorder()
	handler.ServeHTTP(ipaFile, httptest.NewRequest(http.MethodGet, "/files/app-"+beta.FileKey+".ipa", nil))
	if ipaFile.Code != http.StatusOK {
		t.Fatalf("tagged ipa status = %d, body = %s", ipaFile.Code, ipaFile.Body.String())
	}

	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/manifest-"+beta.FileKey+".plist", nil))
	if manifest.Code != http.StatusOK {
		t.Fatalf("tagged manifest status = %d, body = %s", manifest.Code, manifest.Body.String())
	}
	if !strings.Contains(manifest.Body.String(), "com.example.beta") {
		t.Fatalf("tagged manifest missing bundle id: %s", manifest.Body.String())
	}

	install := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/install?tag=beta_1", nil)
	req.Host = "publisher.example.com"
	handler.ServeHTTP(install, req)
	if install.Code != http.StatusFound {
		t.Fatalf("tagged install status = %d", install.Code)
	}
	if got := install.Header().Get("Location"); !strings.Contains(got, "manifest-"+beta.FileKey+".plist") {
		t.Fatalf("install location = %q", got)
	}

	publish := httptest.NewRecorder()
	handler.ServeHTTP(publish, httptest.NewRequest(http.MethodGet, "/api/publish", nil))
	if publish.Code != http.StatusOK {
		t.Fatalf("publish status = %d", publish.Code)
	}
	var payload publishResponse
	decodeResponse(t, publish, &payload)
	if len(payload.Tags) != 2 {
		t.Fatalf("publish tags = %d", len(payload.Tags))
	}
	if !payload.Tags[1].Ready {
		t.Fatalf("beta tag should be ready: %#v", payload.Tags[1])
	}

	doAuthed(t, handler, http.MethodDelete, "/api/tags?tag=beta_1", nil, "")
	if _, err := os.Stat(filepath.Join(app.dataDir, "app-"+beta.FileKey+".ipa")); !os.IsNotExist(err) {
		t.Fatalf("tagged ipa should be deleted, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.dataDir, "manifest-"+beta.FileKey+".plist")); !os.IsNotExist(err) {
		t.Fatalf("tagged plist should be deleted, stat err = %v", err)
	}
}

func TestGeneratePlistAllowsReachableRemoteIPA(t *testing.T) {
	var headCount int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo.ipa" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodHead)
			return
		}
		atomic.AddInt32(&headCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer remote.Close()

	app := testServer(t)
	handler := app.Handler()
	doAuthed(t, handler, http.MethodPost, "/api/config", strings.NewReader(`{
		"appName":"Remote App",
		"releaseNotes":"remote ipa",
		"ipaUrl":"`+remote.URL+`/demo.ipa",
		"plistUrl":"https://publisher.example.com/manifest.plist"
	}`), "application/json")

	doAuthed(t, handler, http.MethodPost, "/api/plist/generate", strings.NewReader(`{
		"bundleIdentifier":"com.example.remote",
		"bundleVersion":"3.0.0",
		"title":"Remote Install"
	}`), "application/json")

	if atomic.LoadInt32(&headCount) != 1 {
		t.Fatalf("HEAD count = %d", headCount)
	}
	if _, err := os.Stat(filepath.Join(app.dataDir, "app.ipa")); !os.IsNotExist(err) {
		t.Fatalf("local ipa should not be required, stat err = %v", err)
	}

	manifest := httptest.NewRecorder()
	handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/manifest.plist", nil))
	if manifest.Code != http.StatusOK {
		t.Fatalf("manifest status = %d, body = %s", manifest.Code, manifest.Body.String())
	}
	if !strings.Contains(manifest.Body.String(), remote.URL+"/demo.ipa") {
		t.Fatalf("manifest missing remote ipa url: %s", manifest.Body.String())
	}

	publish := httptest.NewRecorder()
	handler.ServeHTTP(publish, httptest.NewRequest(http.MethodGet, "/api/publish", nil))
	var payload publishResponse
	decodeResponse(t, publish, &payload)
	if !payload.Ready {
		t.Fatalf("remote publish should be ready: %#v", payload)
	}
}

func TestGeneratePlistStillRequiresDefaultIPAWhenURLIsEmpty(t *testing.T) {
	app := testServer(t)
	handler := app.Handler()

	doAuthed(t, handler, http.MethodPost, "/api/config", strings.NewReader(`{
		"appName":"Demo App",
		"releaseNotes":"",
		"ipaUrl":"",
		"plistUrl":""
	}`), "application/json")

	req := httptest.NewRequest(http.MethodPost, "/api/plist/generate", strings.NewReader(`{
		"bundleIdentifier":"com.example.demo",
		"bundleVersion":"1.0.0",
		"title":"Demo Install"
	}`))
	req.SetBasicAuth("admin", "secret")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "ipa file is required") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestUUIDSearch(t *testing.T) {
	app := testServer(t)
	handler := app.Handler()

	_, err := app.store.UpdateTag(config.DefaultTagName, func(tag *config.Tag) error {
		tag.Analysis = config.Analysis{
			Status:      config.AnalysisSuccess,
			PackageType: config.PackageDevelopment,
			DeviceUUIDs: []string{
				"ABCDEF12-3456-7890-ABCD-EF1234567890",
				"12345678-1234-1234-1234-1234567890AB",
			},
			AnalyzedAt: time.Now().UTC(),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateTag() error = %v", err)
	}

	shortQuery := httptest.NewRecorder()
	handler.ServeHTTP(shortQuery, httptest.NewRequest(http.MethodGet, "/api/uuid/search?q=abc", nil))
	if shortQuery.Code != http.StatusBadRequest {
		t.Fatalf("short query status = %d", shortQuery.Code)
	}

	search := httptest.NewRecorder()
	handler.ServeHTTP(search, httptest.NewRequest(http.MethodGet, "/api/uuid/search?q=ef12", nil))
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", search.Code, search.Body.String())
	}
	var payload uuidSearchResponse
	decodeResponse(t, search, &payload)
	if !payload.Exists || len(payload.Matches) != 1 {
		t.Fatalf("search payload = %#v", payload)
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	return New(Options{
		DataDir:        t.TempDir(),
		Credentials:    auth.Credentials{User: "admin", Password: "secret"},
		MaxUploadBytes: 1024,
		Assets: fstest.MapFS{
			"publish.html":  {Data: []byte("<html>publish</html>")},
			"internal.html": {Data: []byte("<html>internal</html>")},
			"app.js":        {Data: []byte("")},
			"style.css":     {Data: []byte("")},
		},
	})
}

func doAuthed(t *testing.T, handler http.Handler, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	req.SetBasicAuth("admin", "secret")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code < 200 || recorder.Code >= 300 {
		t.Fatalf("%s %s status = %d, body = %s", method, path, recorder.Code, recorder.Body.String())
	}
	return recorder
}

func uploadFakeIPA(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("ipa", "demo.ipa")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("fake ipa")); err != nil {
		t.Fatalf("part.Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}
	return doAuthed(t, handler, http.MethodPost, path, &upload, writer.FormDataContentType())
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), v); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, body = %s", err, recorder.Body.String())
	}
}
