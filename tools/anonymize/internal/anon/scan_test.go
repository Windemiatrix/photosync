package anon

import (
	"encoding/base64"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

var scanWords = map[string]bool{"мырдон": true, "xqzt": true}

func rules(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}

func hasRule(fs []Finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestScanAllowedForms(t *testing.T) {
	t.Parallel()
	doc := `{
 "format": 1,
 "provenance": {"kind": "real-anon", "source": "manual-sync", "expected": "reference-run", "category": "anon-level-same-time-size"},
 "icloud": [{"uuid": "00000000-3F2A-8C41-9D07-5B1E6A0C77F2", "original_filename": "IMG_4821.HEIC",
   "filename": "00000000-3f2a-8c41-ad07-5b1e6a0c77f2.heic", "date": "2052-07-14T18:03:27.455000+03:00",
   "tzoffset": 10800, "original_filesize": 2734118, "live_photo": true, "burst_key": "000000000123456789abcdef01234567",
   "album_info": [{"folder_names": ["2052"], "title": "07/14 Мырдон / Xqzt"}]}],
 "cloud": [{"path": "2052/07-14 - Мырдон/PXL_20520714_183027123 (1).CR2", "kind": "photos_ok", "date_taken": "2052-07-14T18:03:27",
   "sha1": "00000000da39a3ee5e6b4b0d32550000", "live_pair": "cid", "width": 4032}, {"path": "a b c.3gp", "kind": "convert"}],
 "expected": [{"level": "pair_component", "plan": "keep_cloud"}],
 "words": ["мырдон"]
}`
	if fs := Scan("case.json", []byte(doc), scanWords, testSalt); len(fs) != 0 {
		t.Errorf("Scan = %v", fs)
	}
}

func TestScanRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, doc, rule, pointer, value string
	}{
		{"uuid", `{"uuid": "3F2A8C41-9D07-4B1E-8A0C-77F25B1E6A0C"}`, RuleUUID, "/uuid", "3F2A8C41"},
		{"uuid in text", `{"title": "x 3f2a8c41-9d07-4b1e-8a0c-77f25b1e6a0c.heic"}`, RuleUUID, "/title", "3f2a8c41"},
		{"uuid without version 8", `{"burst_key": "00000000-3F2A-4C41-9D07-5B1E6A0C77F2"}`, RuleUUID, "/burst_key", "3F2A"},
		{"hex", `{"cloud": [{"sha1": "da39a3ee5e6b4b0d3255bfef95601890afd80709"}]}`, RuleHex, "/cloud/0/sha1", "da39a3ee"},
		{"coordinate number", `{"size": 55.7539}`, RuleCoordinate, "/size", "55.7539"},
		{"coordinate text", `{"title": "at 55.75393"}`, RuleCoordinate, "/title", "55.75393"},
		{"rounded coordinate text", `{"title": "at 55.75 37.62"}`, RuleCoordinate, "/title", "55.75"},
		{"degree", `{"title": "55°"}`, RuleCoordinate, "/title", "55°"},
		{"email", `{"title": "contact ivan.petrov@example.com"}`, RuleEmail, "/title", "ivan.petrov"},
		{"absolute path", `{"path": "/Users/someone/Pictures"}`, RulePath, "/path", "someone"},
		{"home path", `{"path": "~ivan"}`, RulePath, "/path", "~ivan"},
		{"windows path", `{"path": "C:\\Photos"}`, RulePath, "/path", "Photos"},
		{"windows slash path", `{"path": "D:/Photos"}`, RulePath, "/path", "Photos"},
		{"volumes", `{"path": "x/Volumes/y"}`, RulePath, "/path", "Volumes"},
		{"word", `{"title": "Петров"}`, RuleWord, "/title", "Петров"},
		{"two-letter word", `{"title": "Юл x"}`, RuleWord, "/title", "Юл"},
		{"one-letter runs", `{"title": "Ю x Я"}`, "", "", ""},
		{"word in key", `{"Петров": "Мырдон"}`, RuleUnexpectedKey, "/<key>", "Петров"},
		{"unknown key", `{"pelmeneva": 1}`, RuleUnexpectedKey, "/<key>", "pelmeneva"},
		{"key path", `{"/Users/x": 1}`, RulePath, "/<key>", "/Users/x"},
		{"known keys", `{"cloud": [{"width": 1.5}]}`, RuleCoordinate, "/cloud/0/width", "1.5"},
		{"known key under an unknown one", `{"x": {"title": "Петров"}}`, RuleWord, "/<key>/title", "Петров"},
		{"not json", `{"a": `, RuleNotJSON, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fs := Scan("f.json", []byte(tt.doc), scanWords, nil)
			if tt.rule == "" {
				if len(fs) != 0 {
					t.Errorf("Scan = %v", fs)
				}
				return
			}
			found := false
			for _, f := range fs {
				if f.File != "f.json" {
					t.Errorf("finding file %q", f.File)
				}
				if f.Rule == tt.rule && f.Pointer == tt.pointer {
					found = true
				}
				if tt.value != "" && (strings.Contains(f.Pointer, tt.value) || strings.Contains(f.Rule, tt.value)) {
					t.Errorf("finding %+v holds the value", f)
				}
			}
			if !found {
				t.Errorf("Scan = %v; want %s at %q", fs, tt.rule, tt.pointer)
			}
		})
	}
}

func TestScanSalt(t *testing.T) {
	t.Parallel()
	forms := []string{
		string(testSalt),
		hex.EncodeToString(testSalt),
		strings.ToUpper(hex.EncodeToString(testSalt)),
		base64.StdEncoding.EncodeToString(testSalt),
		base64.RawURLEncoding.EncodeToString(testSalt),
	}
	for _, f := range forms {
		doc := `{"title": "` + strings.ReplaceAll(f, `"`, "") + `"}`
		fs := Scan("f.json", []byte(doc), scanWords, testSalt)
		if !hasRule(fs, RuleSalt) {
			t.Errorf("salt form %q not found: %v", f[:4], rules(fs))
		}
		if fs := Scan("f.json", []byte(doc), scanWords, nil); hasRule(fs, RuleSalt) {
			t.Error("salt rule without a salt")
		}
	}
}

func TestScanDeterministicPointers(t *testing.T) {
	t.Parallel()
	doc := []byte(`{"title": "Петров", "path": "Сидоров", "provenance": {"source": "Иванов", "kind": "Кузнецов"}}`)
	first := Scan("f.json", doc, nil, nil)
	for i := 0; i < 20; i++ {
		if again := Scan("f.json", doc, nil, nil); !reflect.DeepEqual(again, first) {
			t.Fatalf("Scan order changes: %v, %v", first, again)
		}
	}
	var ptrs []string
	for _, f := range first {
		ptrs = append(ptrs, f.Pointer)
	}
	if !reflect.DeepEqual(ptrs, []string{"/path", "/provenance/kind", "/provenance/source", "/title"}) {
		t.Errorf("pointers = %v", ptrs)
	}
}

func TestValidDictionary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tk   Tokens
		ok   bool
	}{
		{"generated words", Tokens{Format: 1, Words: []string{"ghij", "zz", "мыло", "ёжик"}}, true},
		{"empty", Tokens{Format: 1, Words: []string{}}, true},
		{"no words", Tokens{Format: 1}, false},
		{"other format", Tokens{Format: 2, Words: []string{"ghij"}}, false},
		{"not sorted", Tokens{Format: 1, Words: []string{"hh", "gg"}}, false},
		{"repeated", Tokens{Format: 1, Words: []string{"gg", "gg"}}, false},
		{"upper case", Tokens{Format: 1, Words: []string{"Gg"}}, false},
		{"one cluster", Tokens{Format: 1, Words: []string{"g"}}, false},
		{"Latin letter outside the alphabet", Tokens{Format: 1, Words: []string{"zinaida"}}, false},
		{"structural word", Tokens{Format: 1, Words: []string{"mp"}}, false},
		{"digit", Tokens{Format: 1, Words: []string{"g1"}}, false},
	}
	for _, tt := range tests {
		if got := ValidDictionary(tt.tk); got != tt.ok {
			t.Errorf("%s: ValidDictionary = %v; want %v", tt.name, got, tt.ok)
		}
	}
}
