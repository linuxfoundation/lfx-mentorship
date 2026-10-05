// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// fileType is a format identified from a payload's bytes, never from a client's name or header.
type fileType struct {
	contentType string
	extension   string
	matches     func(data []byte) bool
}

var (
	fileTypePNG  = fileType{"image/png", ".png", hasPrefix("\x89PNG\r\n\x1a\n")}
	fileTypeJPEG = fileType{"image/jpeg", ".jpg", hasPrefix("\xff\xd8\xff")}
	fileTypePDF  = fileType{"application/pdf", ".pdf", hasPrefix("%PDF-")}
	fileTypeDOC  = fileType{"application/msword", ".doc", isDOC}
	fileTypeDOCX = fileType{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", isDOCX}
	fileTypeText = fileType{"text/plain; charset=utf-8", ".txt", isPlainText}

	// SVG is excluded: served from S3 it is executable content, a stored-XSS vector.
	logoFileTypes = []fileType{fileTypePNG, fileTypeJPEG}
	// Text is last: it has no signature, so it must not shadow the others.
	taskFileTypes = []fileType{fileTypePDF, fileTypeDOC, fileTypeDOCX, fileTypeText}
	allFileTypes  = []fileType{fileTypePNG, fileTypeJPEG, fileTypePDF, fileTypeDOC, fileTypeDOCX, fileTypeText}
)

const (
	// keyPrefixLength is the "{uuid}-" prefix every object key starts with.
	keyPrefixLength   = 37
	maxFilenameLength = 100
	fallbackFilename  = "file"
	// maxDOCXEntries caps zip.NewReader's per-entry allocation; real documents have far fewer.
	maxDOCXEntries = 1000

	compoundFileSignature = "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
	// Compound-file directory entries are 128 bytes, aligned to the file's 512- or 4096-byte sectors.
	compoundDirEntrySize = 128
	compoundStreamObject = 2
)

// wordDocumentEntryName is "WordDocument" in UTF-16LE with its terminator, as a directory entry stores it.
var wordDocumentEntryName = func() []byte {
	var b []byte
	for _, r := range "WordDocument\x00" {
		b = append(b, byte(r), 0)
	}
	return b
}()

// isDOC requires a WordDocument stream: XLS, PPT and MSI share the compound-file signature but not that stream.
func isDOC(data []byte) bool {
	if !bytes.HasPrefix(data, []byte(compoundFileSignature)) {
		return false
	}
	for off := 512; off+compoundDirEntrySize <= len(data); off += compoundDirEntrySize {
		entry := data[off : off+compoundDirEntrySize]
		nameLength := int(binary.LittleEndian.Uint16(entry[64:66]))
		if nameLength == len(wordDocumentEntryName) && entry[66] == compoundStreamObject && bytes.HasPrefix(entry, wordDocumentEntryName) {
			return true
		}
	}
	return false
}

func hasPrefix(signature string) func([]byte) bool {
	return func(data []byte) bool { return bytes.HasPrefix(data, []byte(signature)) }
}

// isDOCX checks the archive structure, since the ZIP signature alone is shared by many formats.
func isDOCX(data []byte) bool {
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) || bytes.Count(data, []byte("PK\x01\x02")) > maxDOCXEntries {
		return false
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	var contentTypes, document bool
	for _, f := range archive.File {
		switch f.Name {
		case "[Content_Types].xml":
			contentTypes = true
		case "word/document.xml":
			document = true
		}
	}
	return contentTypes && document
}

func isPlainText(data []byte) bool {
	return utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// identifyFileType returns the first allowed type the payload matches.
func identifyFileType(data []byte, allowed []fileType) (fileType, bool) {
	for _, t := range allowed {
		if t.matches(data) {
			return t, true
		}
	}
	return fileType{}, false
}

// fileTypeByContentType looks up a stored object's Content-Type.
func fileTypeByContentType(contentType string) (fileType, bool) {
	for _, t := range allFileTypes {
		if t.contentType == contentType {
			return t, true
		}
	}
	return fileType{}, false
}

func (t fileType) in(types []fileType) bool {
	for _, candidate := range types {
		if candidate.contentType == t.contentType {
			return true
		}
	}
	return false
}

// newObjectKey mints a fresh "{uuid}-{filename}" key; uploads never overwrite.
func newObjectKey(filename string) string {
	return uuid.NewString() + "-" + sanitizeFilename(filename)
}

// sanitizeFilename restricts a name to [A-Za-z0-9._-], capped in length with the extension kept.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := b.String()
	if len(s) > maxFilenameLength {
		ext := path.Ext(s)
		if len(ext) > maxFilenameLength/2 {
			ext = ""
		}
		s = s[:maxFilenameLength-len(ext)] + ext
	}
	if strings.Trim(s, "._-") == "" {
		return fallbackFilename
	}
	return s
}

// downloadFilename derives the Content-Disposition name from a key: the key minus its
// "{uuid}-" prefix, with the extension of the identified type replacing the client's.
func downloadFilename(key string, t fileType) string {
	name := key
	if len(key) > keyPrefixLength && key[keyPrefixLength-1] == '-' {
		if _, err := uuid.Parse(key[:keyPrefixLength-1]); err == nil {
			name = key[keyPrefixLength:]
		}
	}
	name = sanitizeFilename(name)
	base := strings.TrimSuffix(name, path.Ext(name))
	if strings.Trim(base, "._-") == "" {
		base = fallbackFilename
	}
	return base + t.extension
}
