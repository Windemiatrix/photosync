// Package anon holds the pure logic of the anonymize tool: the input and
// output types, the ports of the reference manual-sync script derivations,
// the material consistency checks, the deterministic keyed mapping, the unit
// selection, the relation check and the leak scanner. It does no I/O: the
// command in the parent directory reads and writes the files.
package anon

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Asset is one element of the Photos catalog (osxphotos query --json); field
// names and JSON types are kept so that a case's icloud records are a subset
// of that format. Fields outside this whitelist are never decoded; a null
// decodes as the zero value. burst_key is true on the key photo of a burst;
// burst_photos lists the UUIDs of the other photos of the same burst (empty
// when not a burst).
type Asset struct {
	UUID             string     `json:"uuid"`
	OriginalFilename string     `json:"original_filename"`
	Filename         string     `json:"filename"`
	Date             string     `json:"date"`
	TZOffset         int        `json:"tzoffset"`
	OriginalFilesize int64      `json:"original_filesize"`
	OriginalWidth    int        `json:"original_width"`
	OriginalHeight   int        `json:"original_height"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	InTrash          bool       `json:"intrash,omitempty"`
	HasAdjustments   bool       `json:"hasadjustments"`
	LivePhoto        bool       `json:"live_photo"`
	IsMovie          bool       `json:"ismovie"`
	HasRaw           bool       `json:"has_raw"`
	IsRaw            bool       `json:"israw"`
	Burst            bool       `json:"burst"`
	BurstKeyPhoto    bool       `json:"burst_key"`
	BurstPhotos      []string   `json:"burst_photos"`
	Albums           []AlbumRef `json:"album_info"`
}

// BurstGroup returns the key of the burst of an asset: the smallest UUID of
// the asset and its burst_photos (upper case), or "" when not a burst. Every
// photo of one burst gets the same key.
func (a Asset) BurstGroup() string {
	if !a.Burst {
		return ""
	}
	k := strings.ToUpper(a.UUID)
	for _, u := range a.BurstPhotos {
		if u = strings.ToUpper(u); u < k {
			k = u
		}
	}
	return k
}

// AlbumRef is one album of an asset: the folder path and the title.
type AlbumRef struct {
	FolderNames []string `json:"folder_names"`
	Title       string   `json:"title"`
}

// Name returns the asset name the reference script uses: the original file
// name, or the current one when the original is empty.
func (a Asset) Name() string {
	if a.OriginalFilename != "" {
		return a.OriginalFilename
	}
	return a.Filename
}

// Dim is a frame dimension of a cloud file; 0 means unknown.
type Dim int

// UnmarshalJSON decodes an integer; "" and null are unknown (0).
func (d *Dim) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		*d = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return &json.UnmarshalTypeError{Value: "number", Type: reflect.TypeOf(Dim(0))}
	}
	*d = Dim(n)
	return nil
}

// File is one row of the cloud inventory. Name, stem, ext, folder, year,
// event_month and event_day are not stored: they derive from Path.
type File struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size"`
	DateTaken string `json:"date_taken"`
	Width     Dim    `json:"width"`
	Height    Dim    `json:"height"`
	ContentID string `json:"content_id"`
	LivePair  string `json:"live_pair"`
	SHA1      string `json:"sha1"`
}

// Derived holds the derived columns of one inventory row as the inventory
// wrote them; they are compared with the values derived from the path.
type Derived struct {
	Stem       string `json:"stem"`
	Folder     string `json:"folder"`
	Year       string `json:"year"`
	EventMonth string `json:"event_month"`
}

// Inventory is one version of the cloud inventory: Derived[i] holds the
// derived columns of Files[i].
type Inventory struct {
	Files   []File
	Derived []Derived
}

// Link is one reference link of an asset: a cloud path and the reference
// match kind.
type Link struct {
	Path string
	Kind string
}

// Links maps an asset UUID to its links in the reference order.
type Links map[string][]Link

// PlanDownload is one download row of the reviewed plan.
type PlanDownload struct {
	UUID string `json:"uuid"`
}

// PlanUpload is one upload row of the reviewed plan.
type PlanUpload struct {
	Path string `json:"path"`
}

// PlanReplace is one replace row of the reviewed plan.
type PlanReplace struct {
	UUID       string `json:"uuid"`
	YandexPath string `json:"yandex_path"`
	Match      string `json:"match"`
}

// Plan is the part of the reviewed plan the tool checks.
type Plan struct {
	Download  []PlanDownload `json:"download"`
	Upload    []PlanUpload   `json:"upload"`
	Replace   []PlanReplace  `json:"replace"`
	Conflicts [][]string     `json:"conflicts"`
}

// Material is everything the tool reads: the catalog, the chosen inventory,
// the reviewed plan and the reference links.
type Material struct {
	Assets []Asset
	Inventory
	Plan  Plan
	Links Links
}

// Set is a subset of the material records in material order. Src holds the
// material record index of every record: assets first, then files; a record
// index of a file is len(Material.Assets) plus its inventory index.
type Set struct {
	Assets []Asset
	Files  []File
	Src    []int
}

// Pair is one expected link of a case.
type Pair struct {
	UUID  string `json:"uuid"`
	Path  string `json:"path"`
	Level string `json:"level"`
	Plan  string `json:"plan"`
}

// Provenance tells where a case comes from.
type Provenance struct {
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Expected string `json:"expected"`
	Category string `json:"category"`
}

// Case is one fixture file, testdata/fixtures/<case>/case.json.
type Case struct {
	Format     int        `json:"format"`
	Provenance Provenance `json:"provenance"`
	ICloud     []Asset    `json:"icloud"`
	Cloud      []File     `json:"cloud"`
	Expected   []Pair     `json:"expected"`
}

// Tokens is the dictionary of generated words, tokens.json.
type Tokens struct {
	Format int      `json:"format"`
	Words  []string `json:"words"`
}

// FormatVersion is the version of case.json and tokens.json.
const FormatVersion = 1

// Provenance values of every case.
const (
	ProvenanceKind     = "real-anon"
	ProvenanceSource   = "manual-sync"
	ProvenanceExpected = "reference-run"
)

// Match levels of an expected pair, named as the architecture names them.
const (
	LevelExact         = "exact"
	LevelSameShot      = "same_shot"
	LevelSameShotWeak  = "same_shot_weak"
	LevelSameTimeSize  = "same_time_size"
	LevelProbable      = "probable"
	LevelPairComponent = "pair_component"
)

// RefLiveComponent is the reference name of LevelPairComponent.
const RefLiveComponent = "live_component"

// Levels lists the match levels in output order.
var Levels = []string{LevelExact, LevelSameShot, LevelSameShotWeak, LevelSameTimeSize, LevelProbable, LevelPairComponent}

// refKinds lists the reference link kinds in the order of Levels.
var refKinds = []string{LevelExact, LevelSameShot, LevelSameShotWeak, LevelSameTimeSize, LevelProbable, RefLiveComponent}

// LevelOf converts a reference link kind to a match level; ok is false for an
// unknown kind.
func LevelOf(kind string) (level string, ok bool) {
	i := slices.Index(refKinds, kind)
	if i < 0 {
		return "", false
	}
	return Levels[i], true
}

// Kinds of an inventory row.
const (
	KindPhotosOK = "photos_ok"
	KindConvert  = "convert"
	KindSidecar  = "sidecar"
	KindOther    = "other"
)

// Live pair markers of an inventory row.
const (
	LivePairCID  = "cid"
	LivePairName = "name"
)

// Plan decisions of an expected pair.
const (
	PlanReplaceDecision   = "replace"
	PlanKeepCloudDecision = "keep_cloud"
)

// Conflict types of the reviewed plan the tool checks.
const (
	ConflictProbableMatch = "probable_match"
	ConflictYandexBetter  = "yandex_better"
)

// IsMedia reports whether an inventory kind takes part in matching.
func IsMedia(kind string) bool { return kind == KindPhotosOK || kind == KindConvert }

// structuralWords are letter runs kept as they are (compared without case).
var structuralWords = []string{"img", "dsc", "dscf", "dscn", "pxl", "mvi", "vid", "pano", "burst", "cover", "hdr", "raw", "mp", "edited"}

// knownExts are extensions (lower case, without the dot) kept as they are.
var knownExts = []string{
	"jpg", "jpeg", "heic", "heif", "png", "gif", "tif", "tiff", "webp", "bmp",
	"dng", "cr2", "cr3", "nef", "arw", "raf", "orf", "rw2",
	"mov", "mp4", "m4v", "mpg", "mpeg", "vob", "3gp", "avi", "mts", "m2ts", "wmv", "mkv",
	"xmp", "thm", "aae",
}

// formatStrings are the constant values that appear in the output; their
// letter runs form the format dictionary.
func formatStrings() []string {
	s := []string{
		ProvenanceKind, ProvenanceSource, ProvenanceExpected,
		KindPhotosOK, KindConvert, KindSidecar, KindOther,
		LivePairCID, LivePairName,
		PlanReplaceDecision, PlanKeepCloudDecision,
		"format", "words",
	}
	s = append(s, Levels...)
	for _, c := range Categories {
		s = append(s, "anon-"+c.Name)
	}
	return s
}

var (
	extSet    = runSet(knownExts)
	formatSet = runSet(formatStrings())
)

// runSet returns the lower-case letter runs of the strings.
func runSet(ss []string) map[string]bool {
	l := map[string]bool{}
	for _, s := range ss {
		for _, r := range letterRuns(s) {
			l[lowerNFC(r)] = true
		}
	}
	return l
}

// AllowedWord reports whether a lower-case NFC letter run may appear in the
// output without being a generated word: a structural word, a known
// extension or a word of the format dictionary.
func AllowedWord(w string) bool {
	return slices.Contains(structuralWords, w) || extSet[w] || formatSet[w]
}

// letterRuns returns the letter runs of s: a letter followed by marks.
func letterRuns(s string) []string {
	var runs []string
	for _, r := range splitRuns(s) {
		if r.class == runLetters {
			runs = append(runs, r.text)
		}
	}
	return runs
}

// Run classes of the text rule.
const (
	runOther = iota
	runLetters
	runDigits
)

type textRun struct {
	class int
	text  string
}

// splitRuns cuts s into letter runs (letters with the marks that follow
// them), ASCII digit runs and other runs. A mark that does not follow a
// letter belongs to an other run.
func splitRuns(s string) []textRun {
	var runs []textRun
	start, class := 0, -1
	for i, r := range s {
		c := runOther
		switch {
		case unicode.IsLetter(r):
			c = runLetters
		case unicode.Is(unicode.M, r) && class == runLetters:
			c = runLetters
		case r >= '0' && r <= '9':
			c = runDigits
		}
		if c != class {
			if class >= 0 {
				runs = append(runs, textRun{class, s[start:i]})
			}
			start, class = i, c
		}
	}
	if class >= 0 {
		runs = append(runs, textRun{class, s[start:]})
	}
	return runs
}
