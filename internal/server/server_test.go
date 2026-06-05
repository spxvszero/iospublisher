package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"iospublisher/internal/auth"
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
	doAuthed(t, handler, http.MethodPost, "/api/upload", &upload, writer.FormDataContentType())

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
