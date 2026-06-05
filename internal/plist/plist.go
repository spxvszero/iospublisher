package plist

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strings"

	"iospublisher/internal/config"
)

type GenerateInput struct {
	BundleIdentifier string `json:"bundleIdentifier"`
	BundleVersion    string `json:"bundleVersion"`
	Title            string `json:"title"`
}

func Generate(cfg config.Config, input GenerateInput) ([]byte, error) {
	ipaURL := strings.TrimSpace(cfg.IPAURL)
	bundleID := strings.TrimSpace(input.BundleIdentifier)
	bundleVersion := strings.TrimSpace(input.BundleVersion)
	title := strings.TrimSpace(input.Title)

	switch {
	case ipaURL == "":
		return nil, errors.New("ipa url is required")
	case bundleID == "":
		return nil, errors.New("bundle identifier is required")
	case bundleVersion == "":
		return nil, errors.New("bundle version is required")
	case title == "":
		return nil, errors.New("plist title is required")
	}

	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString(`<dict>` + "\n")
	b.WriteString(`  <key>items</key>` + "\n")
	b.WriteString(`  <array>` + "\n")
	b.WriteString(`    <dict>` + "\n")
	b.WriteString(`      <key>assets</key>` + "\n")
	b.WriteString(`      <array>` + "\n")
	b.WriteString(`        <dict>` + "\n")
	b.WriteString(`          <key>kind</key>` + "\n")
	b.WriteString(`          <string>software-package</string>` + "\n")
	b.WriteString(`          <key>url</key>` + "\n")
	writeString(&b, ipaURL, 10)
	b.WriteString(`        </dict>` + "\n")
	b.WriteString(`      </array>` + "\n")
	b.WriteString(`      <key>metadata</key>` + "\n")
	b.WriteString(`      <dict>` + "\n")
	b.WriteString(`        <key>bundle-identifier</key>` + "\n")
	writeString(&b, bundleID, 8)
	b.WriteString(`        <key>bundle-version</key>` + "\n")
	writeString(&b, bundleVersion, 8)
	b.WriteString(`        <key>kind</key>` + "\n")
	b.WriteString(`        <string>software</string>` + "\n")
	b.WriteString(`        <key>title</key>` + "\n")
	writeString(&b, title, 8)
	b.WriteString(`      </dict>` + "\n")
	b.WriteString(`    </dict>` + "\n")
	b.WriteString(`  </array>` + "\n")
	b.WriteString(`</dict>` + "\n")
	b.WriteString(`</plist>` + "\n")
	return b.Bytes(), nil
}

func writeString(b *bytes.Buffer, value string, indent int) {
	for i := 0; i < indent; i++ {
		b.WriteByte(' ')
	}
	b.WriteString("<string>")
	_ = xml.EscapeText(b, []byte(value))
	b.WriteString("</string>\n")
}
