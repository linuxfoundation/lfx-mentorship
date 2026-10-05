// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package service

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func zipWith(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range names {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write([]byte("<x/>")); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// compoundFileWith builds a minimal compound file whose directory holds the named entries (type 2 is a stream).
func compoundFileWith(entries ...compoundEntry) []byte {
	data := make([]byte, 512, 512+compoundDirEntrySize*(len(entries)+1))
	copy(data, compoundFileSignature)
	for _, e := range append([]compoundEntry{{"Root Entry", 5}}, entries...) {
		entry := make([]byte, compoundDirEntrySize)
		name := []rune(e.name + "\x00")
		for i, r := range name {
			binary.LittleEndian.PutUint16(entry[i*2:], uint16(r))
		}
		binary.LittleEndian.PutUint16(entry[64:], uint16(len(name)*2))
		entry[66] = e.objectType
		data = append(data, entry...)
	}
	return data
}

type compoundEntry struct {
	name       string
	objectType byte
}

func TestIdentifyFileType(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		allowed []fileType
		want    string
	}{
		{"png logo", []byte("\x89PNG\r\n\x1a\nrest"), logoFileTypes, "image/png"},
		{"jpeg logo", []byte("\xff\xd8\xff\xe0rest"), logoFileTypes, "image/jpeg"},
		{"svg logo rejected", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`), logoFileTypes, ""},
		{"pdf as logo rejected", []byte("%PDF-1.7"), logoFileTypes, ""},
		{"pdf", []byte("%PDF-1.7\n\x00\x01"), taskFileTypes, "application/pdf"},
		{"doc", compoundFileWith(compoundEntry{"\x05SummaryInformation", 2}, compoundEntry{"WordDocument", 2}), taskFileTypes, "application/msword"},
		{"xls rejected", compoundFileWith(compoundEntry{"Workbook", 2}), taskFileTypes, ""},
		{"ppt rejected", compoundFileWith(compoundEntry{"PowerPoint Document", 2}), taskFileTypes, ""},
		{"msi rejected", compoundFileWith(compoundEntry{"\x05SummaryInformation", 2}, compoundEntry{"\u4840\u3f3f\u4577", 2}), taskFileTypes, ""},
		{"WordDocument storage rejected", compoundFileWith(compoundEntry{"WordDocument", 1}), taskFileTypes, ""},
		{"compound header alone rejected", []byte(compoundFileSignature + "\x00\x00"), taskFileTypes, ""},
		{"docx", zipWith(t, "[Content_Types].xml", "word/document.xml"), taskFileTypes, fileTypeDOCX.contentType},
		{"plain zip rejected", zipWith(t, "[Content_Types].xml", "xl/workbook.xml"), taskFileTypes, ""},
		{"text", []byte("func main() {}\n"), taskFileTypes, fileTypeText.contentType},
		{"binary with NUL rejected", []byte("MZ\x90\x00\x03"), taskFileTypes, ""},
		{"invalid utf-8 rejected", []byte{0xff, 0xfe, 0xfd}, taskFileTypes, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := identifyFileType(tc.data, tc.allowed)
			if tc.want == "" {
				if ok {
					t.Fatalf("identified %q; want rejection", got.contentType)
				}
				return
			}
			if !ok || got.contentType != tc.want {
				t.Fatalf("got %q (ok=%v); want %q", got.contentType, ok, tc.want)
			}
		})
	}
}

func TestIsDOCX_RejectsOversizedDirectory(t *testing.T) {
	names := []string{"[Content_Types].xml", "word/document.xml"}
	for i := range maxDOCXEntries {
		names = append(names, fmt.Sprintf("word/media/%d.xml", i))
	}
	if isDOCX(zipWith(t, names...)) {
		t.Fatal("an archive with more entries than a document has must be rejected before parsing")
	}
}

func TestSanitizeFilename(t *testing.T) {
	long := strings.Repeat("a", 150) + ".pdf"
	tests := map[string]string{
		"report.pdf":            "report.pdf",
		"my report (final).pdf": "my_report__final_.pdf",
		"../../etc/passwd":      ".._.._etc_passwd",
		"résumé.docx":           "r_sum_.docx",
		"":                      fallbackFilename,
		"...":                   fallbackFilename,
		long:                    strings.Repeat("a", maxFilenameLength-4) + ".pdf",
	}
	for in, want := range tests {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestNewObjectKey(t *testing.T) {
	key := newObjectKey("My File.pdf")
	if _, err := uuid.Parse(key[:keyPrefixLength-1]); err != nil || key[keyPrefixLength-1] != '-' {
		t.Fatalf("key %q does not start with {uuid}-", key)
	}
	if got := key[keyPrefixLength:]; got != "My_File.pdf" {
		t.Fatalf("key name = %q; want My_File.pdf", got)
	}
	if other := newObjectKey("My File.pdf"); other == key {
		t.Fatal("keys must be fresh per upload")
	}
}

func TestDownloadFilename(t *testing.T) {
	id := uuid.NewString()
	tests := []struct {
		key  string
		t    fileType
		want string
	}{
		{id + "-essay.pdf", fileTypePDF, "essay.pdf"},
		{id + "-x.bat", fileTypeText, "x.txt"},
		{id + "-setup.msi", fileTypeDOC, "setup.doc"},
		{id + "-.pdf", fileTypePDF, "file.pdf"},
		{id + "-my notes.txt", fileTypeText, "my_notes.txt"},
		{"legacy name.doc", fileTypeDOC, "legacy_name.doc"},
		{id + "-blob", fileType{contentType: octetStream}, "blob"},
	}
	for _, tc := range tests {
		if got := downloadFilename(tc.key, tc.t); got != tc.want {
			t.Errorf("downloadFilename(%q) = %q; want %q", tc.key, got, tc.want)
		}
	}
}
