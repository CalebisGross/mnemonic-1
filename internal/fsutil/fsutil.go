// Package fsutil provides filesystem utility functions used by encoding and ingest.
package fsutil

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// IsSensitiveFile checks if a file path matches any sensitive pattern.
func IsSensitiveFile(path string, patterns []string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, pattern := range patterns {
		p := strings.ToLower(pattern)
		if strings.Contains(base, p) {
			return true
		}
	}
	return false
}

// MatchesExcludePattern checks if a path matches any exclude pattern.
func MatchesExcludePattern(path string, patterns []string) bool {
	pathWithSlash := path + "/"
	for _, pattern := range patterns {
		if strings.Contains(path, pattern) || strings.Contains(pathWithSlash, pattern) {
			return true
		}
	}
	return false
}

// IsBinaryFile checks if a file is a binary file based on extension.
func IsBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".bin": true, ".o": true, ".so": true,
		".dylib": true, ".a": true, ".dll": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".bmp": true, ".ico": true, ".tiff": true,
		".heic": true, ".heif": true, ".raw": true,
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".m4a": true,
		".zip": true, ".gz": true, ".tar": true, ".bz2": true,
		".xz": true, ".7z": true, ".rar": true, ".dmg": true,
		".pdf": true, ".doc": true, ".xls": true, ".ppt": true,
		".docx": true, ".xlsx": true, ".pptx": true,
		".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
		".sqlite": true, ".sqlite-shm": true, ".sqlite-wal": true,
		".db": true, ".db-shm": true, ".db-wal": true,
		".ldb": true, ".sst": true, ".log": true,
		".class": true, ".pyc": true, ".pyo": true,
		".photoslibrary": true, ".dict": true, ".data": true,
	}
	return binaryExts[ext]
}

// IsBinaryContent checks if content appears to be binary by looking for
// non-printable bytes in the first 512 bytes.
func IsBinaryContent(content string) bool {
	checkLen := len(content)
	if checkLen > 512 {
		checkLen = 512
	}
	if checkLen == 0 {
		return false
	}
	nonPrintable := 0
	for i := 0; i < checkLen; i++ {
		b := content[i]
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			nonPrintable++
		}
	}
	return float64(nonPrintable)/float64(checkLen) > 0.10
}

// ReadFileContent reads the first maxBytes of a file.
func ReadFileContent(path string, maxBytes int, log *slog.Logger) string {
	file, err := os.Open(path)
	if err != nil {
		log.Debug("failed to open file for reading", "path", path, "err", err)
		return ""
	}
	defer func() { _ = file.Close() }()

	limitedReader := io.LimitReader(file, int64(maxBytes))
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Debug("failed to read file content", "path", path, "err", err)
		return ""
	}

	var nextByte [1]byte
	if n, err := file.Read(nextByte[:]); err == nil && n > 0 {
		return string(content) + "\n... [truncated]"
	}

	return string(content)
}
