package ipa

import (
	"archive/zip"
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"iospublisher/internal/config"
)

const maxProfileBytes = 20 << 20

type profileInfo struct {
	ProvisionedDevices    []string
	ProvisionsAllDevices  bool
	GetTaskAllow          bool
	DeveloperCertificates [][]byte
	ExpirationDate        time.Time
}

func Analyze(path string) config.Analysis {
	now := time.Now().UTC()
	profile, err := readProvisioningProfile(path)
	if err != nil {
		return failedAnalysis(now, err)
	}
	info, err := parseProfile(profile)
	if err != nil {
		return failedAnalysis(now, err)
	}

	analysis := config.Analysis{
		Status:           config.AnalysisSuccess,
		PackageType:      packageType(info),
		DeviceUUIDs:      info.ProvisionedDevices,
		ProfileExpiresAt: info.ExpirationDate,
		AnalyzedAt:       now,
	}
	analysis.CertificateExpiresAt = earliestCertificateExpiry(info.DeveloperCertificates)
	return analysis
}

func failedAnalysis(now time.Time, err error) config.Analysis {
	return config.Analysis{
		Status:      config.AnalysisFailed,
		PackageType: config.PackageUnknown,
		DeviceUUIDs: []string{},
		AnalyzedAt:  now,
		Error:       err.Error(),
	}
}

func readProvisioningProfile(path string) ([]byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		name := filepath.ToSlash(file.Name)
		if !strings.HasPrefix(name, "Payload/") || !strings.HasSuffix(name, ".app/embedded.mobileprovision") {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(handle, maxProfileBytes+1))
		closeErr := handle.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > maxProfileBytes {
			return nil, errors.New("provisioning profile is too large")
		}
		return extractXMLPlist(data)
	}
	return nil, errors.New("embedded.mobileprovision not found")
}

func extractXMLPlist(data []byte) ([]byte, error) {
	start := bytes.Index(data, []byte("<?xml"))
	if start < 0 {
		start = bytes.Index(data, []byte("<plist"))
	}
	end := bytes.LastIndex(data, []byte("</plist>"))
	if start < 0 || end < 0 || end < start {
		return nil, errors.New("plist payload not found in provisioning profile")
	}
	end += len("</plist>")
	return data[start:end], nil
}

func parseProfile(data []byte) (profileInfo, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return profileInfo{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "dict" {
			continue
		}
		root, err := parseDict(decoder)
		if err != nil {
			return profileInfo{}, err
		}
		return profileFromPlist(root), nil
	}
}

func profileFromPlist(root map[string]any) profileInfo {
	entitlements, _ := root["Entitlements"].(map[string]any)
	return profileInfo{
		ProvisionedDevices:    stringSlice(root["ProvisionedDevices"]),
		ProvisionsAllDevices:  boolValue(root["ProvisionsAllDevices"]),
		GetTaskAllow:          boolValue(entitlements["get-task-allow"]),
		DeveloperCertificates: dataSlice(root["DeveloperCertificates"]),
		ExpirationDate:        timeValue(root["ExpirationDate"]),
	}
}

func packageType(info profileInfo) string {
	switch {
	case info.ProvisionsAllDevices:
		return config.PackageEnterprise
	case len(info.ProvisionedDevices) > 0 && info.GetTaskAllow:
		return config.PackageDevelopment
	case len(info.ProvisionedDevices) > 0:
		return config.PackageAdHoc
	default:
		return config.PackageAppStore
	}
}

func earliestCertificateExpiry(certificates [][]byte) time.Time {
	var earliest time.Time
	for _, data := range certificates {
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			continue
		}
		if earliest.IsZero() || cert.NotAfter.Before(earliest) {
			earliest = cert.NotAfter.UTC()
		}
	}
	return earliest
}

func parseDict(decoder *xml.Decoder) (map[string]any, error) {
	result := map[string]any{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			if typed.Name.Local != "key" {
				return nil, fmt.Errorf("expected plist key, got %s", typed.Name.Local)
			}
			key, err := readText(decoder, "key")
			if err != nil {
				return nil, err
			}
			start, err := nextStart(decoder)
			if err != nil {
				return nil, err
			}
			value, err := parseValue(decoder, start)
			if err != nil {
				return nil, err
			}
			result[key] = value
		case xml.EndElement:
			if typed.Name.Local == "dict" {
				return result, nil
			}
		}
	}
}

func parseArray(decoder *xml.Decoder) ([]any, error) {
	var result []any
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			value, err := parseValue(decoder, typed)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		case xml.EndElement:
			if typed.Name.Local == "array" {
				return result, nil
			}
		}
	}
}

func parseValue(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		return parseDict(decoder)
	case "array":
		return parseArray(decoder)
	case "string":
		return readText(decoder, "string")
	case "date":
		text, err := readText(decoder, "date")
		if err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(text))
		if err != nil {
			return time.Time{}, nil
		}
		return parsed.UTC(), nil
	case "data":
		text, err := readText(decoder, "data")
		if err != nil {
			return nil, err
		}
		decoded, err := base64.StdEncoding.DecodeString(stripWhitespace(text))
		if err != nil {
			return []byte{}, nil
		}
		return decoded, nil
	case "true":
		return true, consumeEnd(decoder, "true")
	case "false":
		return false, consumeEnd(decoder, "false")
	default:
		if err := consumeEnd(decoder, start.Name.Local); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func nextStart(decoder *xml.Decoder) (xml.StartElement, error) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if start, ok := token.(xml.StartElement); ok {
			return start, nil
		}
	}
}

func readText(decoder *xml.Decoder, endName string) (string, error) {
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch typed := token.(type) {
		case xml.CharData:
			b.Write([]byte(typed))
		case xml.EndElement:
			if typed.Name.Local == endName {
				return b.String(), nil
			}
		}
	}
}

func consumeEnd(decoder *xml.Decoder, endName string) error {
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == endName {
			return nil
		}
	}
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return []string{}
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			result = append(result, text)
		}
	}
	return result
}

func dataSlice(value any) [][]byte {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([][]byte, 0, len(items))
	for _, item := range items {
		data, ok := item.([]byte)
		if ok && len(data) > 0 {
			result = append(result, data)
		}
	}
	return result
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func timeValue(value any) time.Time {
	typed, _ := value.(time.Time)
	return typed
}

func stripWhitespace(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r', '\t':
			return -1
		default:
			return r
		}
	}, value)
}
