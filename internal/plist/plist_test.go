package plist

import (
	"strings"
	"testing"

	"iospublisher/internal/config"
)

func TestGenerateEscapesValues(t *testing.T) {
	data, err := Generate(config.Config{
		AppName: "Demo & QA",
		IPAURL:  "https://example.com/app.ipa?channel=a&b=c",
	}, GenerateInput{
		BundleIdentifier: "com.example.demo",
		BundleVersion:    "1.2.3",
		Title:            "Install & Demo",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"<key>items</key>",
		"<string>software-package</string>",
		"<string>https://example.com/app.ipa?channel=a&amp;b=c</string>",
		"<string>com.example.demo</string>",
		"<string>1.2.3</string>",
		"<string>Install &amp; Demo</string>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plist missing %q\n%s", want, got)
		}
	}
}

func TestGenerateValidatesInput(t *testing.T) {
	_, err := Generate(config.Config{AppName: "Demo", IPAURL: "https://example.com/app.ipa"}, GenerateInput{})
	if err == nil {
		t.Fatal("Generate() expected validation error")
	}
}
