package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// Input files the tool opens, relative to --in. Nothing else is opened and
// the directory is never listed.
const (
	nameCatalog       = "icloud_catalog.json"
	nameInventory     = "yandex_inventory.json"
	namePrevInventory = "yandex_inventory.prev.json"
	namePlan          = "plan/plan.json"
	nameRerunPlan     = "plan-rerun/plan.json"
	nameLinks         = "plan-rerun/links.json"
	nameTokens        = "tokens.json"
)

// Stages of an error.
const (
	stageIn        = "in dir"
	stageOut       = "out dir"
	stageSalt      = "salt"
	stageCatalog   = "read catalog"
	stageInventory = "read inventory"
	stagePlan      = "read plan"
	stageRerun     = "plan rerun"
	stageLinks     = "read links"
	stagePartition = "partition"
	stageRelations = "relations"
	stageBuild     = "build"
	stageWrite     = "write"
	stageCheckDir  = "check dir"
	stageCheck     = "check"
)

// Kinds of an error. Kinds with a field or a number are built by the
// functions below from code constants and integers only.
const (
	kindNotFound      = "not found"
	kindPermission    = "permission denied"
	kindNotDir        = "not a directory"
	kindNotEmpty      = "not empty"
	kindInsideIn      = "inside --in"
	kindInsideOut     = "inside --out"
	kindInsideGit     = "inside a git working tree"
	kindNotRegular    = "not a regular file"
	kindOpenMode      = "permissions too open"
	kindTooLarge      = "too large"
	kindNotHex        = "not 64 lower-case hex digits"
	kindRerunDiffers  = "plan rerun differs from the reviewed plan"
	kindUnsupported   = "unsupported value"
	kindExhausted     = "mapping space exhausted"
	kindUnexpected    = "unexpected error"
	kindUnexpectedEOF = "unexpected end of input"
	kindTrailing      = "trailing data"
)

func kindSyntax(offset int64) string {
	return "syntax error at offset " + strconv.FormatInt(offset, 10)
}

func kindWrongType(field string) string { return "wrong type in field " + field }

func kindMissing(field string, record int) string {
	return "missing field " + field + " in record " + strconv.Itoa(record)
}
func kindInvalid(field string, record int) string {
	return "invalid value in field " + field + " in record " + strconv.Itoa(record)
}

// toolError is an error of the tool: a stage and a kind, both built from
// code constants and numbers only. Errors of other packages are translated
// into it at the boundary and never printed themselves (they may hold paths
// or input literals).
type toolError struct{ stage, kind string }

func (e *toolError) Error() string { return e.stage + ": " + e.kind }

func fail(stage, kind string) *toolError { return &toolError{stage, kind} }

// knownFields are the JSON paths a type error may name.
var knownFields = map[string]bool{}

func init() {
	for _, f := range []string{
		"uuid", "original_filename", "filename", "date", "tzoffset", "original_filesize", "original_width",
		"original_height", "width", "height", "intrash", "hasadjustments", "live_photo", "ismovie", "has_raw",
		"israw", "burst", "burst_key", "album_info", "album_info.folder_names", "album_info.title",
		"files", "files.path", "files.kind", "files.size", "files.date_taken", "files.content_id",
		"files.live_pair", "files.sha1", "files.stem", "files.folder", "files.year", "files.event_month",
		"download", "download.uuid", "upload", "upload.path", "replace", "replace.uuid", "replace.yandex_path",
		"replace.match", "conflicts",
	} {
		knownFields[f] = true
	}
}

// fieldPath drops the array indices of a JSON field path: "files.0.size"
// becomes "files.size".
func fieldPath(p string) string {
	parts := strings.Split(p, ".")
	out := parts[:0]
	for _, s := range parts {
		if _, err := strconv.Atoi(s); err != nil {
			out = append(out, s)
		}
	}
	return strings.Join(out, ".")
}

// translate converts an error to a toolError of the stage; field names a
// type error whose own field is not a known JSON path.
func translate(stage string, err error, field string) *toolError {
	var te *toolError
	var se *json.SyntaxError
	var ue *json.UnmarshalTypeError
	switch {
	case errors.As(err, &te):
		return te
	case errors.As(err, &se):
		return fail(stage, kindSyntax(se.Offset))
	case errors.As(err, &ue):
		f := fieldPath(ue.Field)
		if !knownFields[f] {
			f = field
		}
		return fail(stage, kindWrongType(f))
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return fail(stage, kindUnexpectedEOF)
	case errors.Is(err, anon.ErrTrailingData):
		return fail(stage, kindTrailing)
	case errors.Is(err, fs.ErrNotExist):
		return fail(stage, kindNotFound)
	case errors.Is(err, fs.ErrPermission):
		return fail(stage, kindPermission)
	case errors.Is(err, anon.ErrUnsupported):
		return fail(stage, kindUnsupported)
	case errors.Is(err, anon.ErrExhausted):
		return fail(stage, kindExhausted)
	}
	return fail(stage, kindUnexpected)
}

// closeQuietly closes a file or a root that holds no unwritten data: a close
// error loses nothing there.
func closeQuietly(c io.Closer) { _ = c.Close() }

// decodeFile decodes one JSON value of an input file and rejects trailing
// data.
func decodeFile(fsys fs.FS, name string, v any, stage, field string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return translate(stage, err, field)
	}
	defer closeQuietly(f)
	dec := json.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	if err := dec.Decode(v); err != nil {
		return translate(stage, err, field)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fail(stage, kindTrailing)
	}
	return nil
}

func readAll(fsys fs.FS, name, stage string) ([]byte, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, translate(stage, err, "")
	}
	return data, nil
}

// readCatalog streams the catalog array element by element into the
// whitelisted asset type; the file is never held in memory as a whole.
func readCatalog(fsys fs.FS) ([]anon.Asset, error) {
	f, err := fsys.Open(nameCatalog)
	if err != nil {
		return nil, translate(stageCatalog, err, "catalog")
	}
	defer closeQuietly(f)
	dec := json.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	tok, err := dec.Token()
	if err != nil {
		return nil, translate(stageCatalog, err, "catalog")
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fail(stageCatalog, kindWrongType("catalog"))
	}
	var out []anon.Asset
	for n := 1; dec.More(); n++ {
		var a anon.Asset
		if err := dec.Decode(&a); err != nil {
			return nil, translate(stageCatalog, err, "catalog")
		}
		if a.UUID == "" {
			return nil, fail(stageCatalog, kindMissing("uuid", n))
		}
		out = append(out, a)
	}
	if _, err := dec.Token(); err != nil {
		return nil, translate(stageCatalog, err, "catalog")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fail(stageCatalog, kindTrailing)
	}
	return out, nil
}

// inventoryRow is one row of the inventory with the derived columns the
// tool compares; every other column (root, created, camera, mtime, ...) is
// never decoded.
type inventoryRow struct {
	Path       string          `json:"path"`
	Kind       string          `json:"kind"`
	Size       int64           `json:"size"`
	DateTaken  string          `json:"date_taken"`
	Width      json.RawMessage `json:"width"`
	Height     json.RawMessage `json:"height"`
	ContentID  string          `json:"content_id"`
	LivePair   string          `json:"live_pair"`
	SHA1       string          `json:"sha1"`
	Stem       string          `json:"stem"`
	Folder     string          `json:"folder"`
	Year       string          `json:"year"`
	EventMonth string          `json:"event_month"`
}

// parseDim parses a frame dimension: a JSON integer, "" or null (0).
func parseDim(raw json.RawMessage, field string) (anon.Dim, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == `""` {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New(kindWrongType(field))
	}
	return anon.Dim(n), nil
}

// readInventory reads one inventory; an optional one that does not exist
// gives nil.
func readInventory(fsys fs.FS, name string, optional bool) (*anon.Inventory, error) {
	var doc struct {
		Files *[]inventoryRow `json:"files"`
	}
	if err := decodeFile(fsys, name, &doc, stageInventory, "files"); err != nil {
		var te *toolError
		if optional && errors.As(err, &te) && te.kind == kindNotFound {
			return nil, nil
		}
		return nil, err
	}
	if doc.Files == nil {
		return nil, fail(stageInventory, "missing field files")
	}
	inv := &anon.Inventory{}
	for i, r := range *doc.Files {
		n := i + 1
		switch {
		case r.Path == "":
			return nil, fail(stageInventory, kindMissing("files.path", n))
		case strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, `\`):
			return nil, fail(stageInventory, kindInvalid("files.path", n))
		case r.LivePair != "" && r.LivePair != anon.LivePairCID && r.LivePair != anon.LivePairName:
			// The value is copied into case.json: only the known markers.
			return nil, fail(stageInventory, kindInvalid("files.live_pair", n))
		}
		w, err := parseDim(r.Width, "files.width")
		if err != nil {
			return nil, fail(stageInventory, err.Error())
		}
		h, err := parseDim(r.Height, "files.height")
		if err != nil {
			return nil, fail(stageInventory, err.Error())
		}
		inv.Files = append(inv.Files, anon.File{Path: r.Path, Kind: r.Kind, Size: r.Size, DateTaken: r.DateTaken,
			Width: w, Height: h, ContentID: r.ContentID, LivePair: r.LivePair, SHA1: r.SHA1})
		inv.Derived = append(inv.Derived, anon.Derived{Stem: r.Stem, Folder: r.Folder, Year: r.Year, EventMonth: r.EventMonth})
	}
	return inv, nil
}

// readLinks reads the reference links of the rerun.
func readLinks(fsys fs.FS) (anon.Links, error) {
	var raw map[string][][]string
	if err := decodeFile(fsys, nameLinks, &raw, stageLinks, "links"); err != nil {
		return nil, err
	}
	links := anon.Links{}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for n, k := range keys {
		for _, l := range raw[k] {
			if len(l) != 2 {
				return nil, fail(stageLinks, kindWrongType("links"))
			}
			if _, ok := anon.LevelOf(l[1]); !ok {
				return nil, fail(stageLinks, kindInvalid("links.kind", n+1))
			}
			links[k] = append(links[k], anon.Link{Path: l[0], Kind: l[1]})
		}
		if links[k] == nil {
			links[k] = []anon.Link{}
		}
	}
	return links, nil
}

// readMaterial reads the material from the input directory: the reviewed
// plan and its rerun (they must be equal), the reference links, the catalog
// and the first inventory that passes the partition check. It returns which
// inventory was taken: "current" or "prev".
func readMaterial(fsys fs.FS) (*anon.Material, string, error) {
	reviewed, err := readAll(fsys, namePlan, stagePlan)
	if err != nil {
		return nil, "", err
	}
	var plan anon.Plan
	dec := json.NewDecoder(bytes.NewReader(reviewed))
	if err := dec.Decode(&plan); err != nil {
		return nil, "", translate(stagePlan, err, "plan")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, "", fail(stagePlan, kindTrailing)
	}
	rerun, err := readAll(fsys, nameRerunPlan, stageRerun)
	if err != nil {
		return nil, "", err
	}
	same, err := anon.SamePlan(reviewed, rerun)
	if err != nil {
		return nil, "", translate(stageRerun, err, "plan")
	}
	if !same {
		return nil, "", fail(stageRerun, kindRerunDiffers)
	}
	links, err := readLinks(fsys)
	if err != nil {
		return nil, "", err
	}
	assets, err := readCatalog(fsys)
	if err != nil {
		return nil, "", err
	}
	m := &anon.Material{Assets: assets, Plan: plan, Links: links}
	picked, attempts, err := anon.PickInventory(m, func(i int) (*anon.Inventory, error) {
		if i == 0 {
			return readInventory(fsys, nameInventory, false)
		}
		return readInventory(fsys, namePrevInventory, true)
	})
	if errors.Is(err, anon.ErrPartition) {
		parts := make([]string, len(attempts))
		for i, c := range attempts {
			parts[i] = []string{"current", "prev"}[i] + ": " + c.String()
		}
		return nil, "", fail(stagePartition, "no inventory passes the check: "+strings.Join(parts, "; "))
	}
	if err != nil {
		return nil, "", translate(stageInventory, err, "files")
	}
	return m, []string{"current", "prev"}[picked], nil
}

// realPath returns the absolute path with symbolic links resolved; for a
// path that does not exist yet, the longest existing parent is resolved.
func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var rest []string
	for cur := abs; ; {
		r, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{r}, rest...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = parent
	}
}

// within reports whether child is parent or lies below it.
func within(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// inGitTree reports whether dir or one of its parents holds .git.
func inGitTree(dir string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

var saltRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// loadSalt reads the salt file (64 lower-case hex digits and a newline,
// mode 0600 or stricter) or, when create is set and the file is missing,
// creates it with 32 random bytes.
func loadSalt(path string, create bool) (salt []byte, created bool, err error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if !create {
			return nil, false, fail(stageSalt, kindNotFound)
		}
		salt, err := createSalt(path)
		return salt, err == nil, err
	}
	if err != nil {
		return nil, false, translate(stageSalt, err, "")
	}
	switch {
	case !st.Mode().IsRegular():
		return nil, false, fail(stageSalt, kindNotRegular)
	case st.Mode().Perm()&0o077 != 0:
		return nil, false, fail(stageSalt, kindOpenMode)
	case st.Size() > 128:
		return nil, false, fail(stageSalt, kindTooLarge)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the path is the --salt-file flag of the developer running the tool
	if err != nil {
		return nil, false, translate(stageSalt, err, "")
	}
	s := strings.TrimSuffix(string(data), "\n")
	if !saltRE.MatchString(s) {
		return nil, false, fail(stageSalt, kindNotHex)
	}
	salt, err = hex.DecodeString(s)
	if err != nil {
		return nil, false, fail(stageSalt, kindNotHex)
	}
	return salt, false, nil
}

func createSalt(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, translate(stageSalt, err, "")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the path is the --salt-file flag of the developer running the tool
	if err != nil {
		return nil, translate(stageSalt, err, "")
	}
	salt := make([]byte, 32)
	_, _ = rand.Read(salt) // crypto/rand.Read never returns an error
	_, werr := f.WriteString(hex.EncodeToString(salt) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return nil, translate(stageSalt, werr, "")
	}
	return salt, nil
}

// encodeJSON encodes a value as the output files are written: one space of
// indent, no HTML escaping, UTF-8 as it is, a newline at the end.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// openOut creates out when it is missing (os.Mkdir fails on anything that
// appeared there) and opens it as a root. Right before writing it checks
// again that out is a real directory, not a symbolic link, that the opened
// root is that directory and that it is empty; the mode is set through the
// root, so nothing outside out is touched.
func openOut(out string) (*os.Root, error) {
	if _, err := os.Lstat(out); errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(out, 0o700); err != nil {
			return nil, translate(stageWrite, err, "")
		}
	}
	st, err := os.Lstat(out)
	if err != nil {
		return nil, translate(stageOut, err, "")
	}
	if !st.IsDir() {
		return nil, fail(stageOut, kindNotDir)
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		return nil, translate(stageWrite, err, "")
	}
	if err := prepareOut(root, st); err != nil {
		closeQuietly(root)
		return nil, err
	}
	return root, nil
}

// prepareOut checks that root is the directory st describes and is empty,
// then sets its mode to 0700.
func prepareOut(root *os.Root, st fs.FileInfo) error {
	rst, err := root.Stat(".")
	if err != nil {
		return translate(stageWrite, err, "")
	}
	if !os.SameFile(st, rst) {
		return fail(stageOut, kindNotDir)
	}
	entries, err := fs.ReadDir(root.FS(), ".")
	switch {
	case err != nil:
		return translate(stageWrite, err, "")
	case len(entries) > 0:
		return fail(stageOut, kindNotEmpty)
	}
	if err := root.Chmod(".", 0o700); err != nil {
		return translate(stageWrite, err, "")
	}
	return nil
}

// writeOutput writes tokens.json and one directory per non-empty case into
// out (created when missing): directories 0700, files 0600, never over an
// existing file.
func writeOutput(out string, res *anon.Result) error {
	root, err := openOut(out)
	if err != nil {
		return err
	}
	defer closeQuietly(root)
	words := res.Words
	if words == nil {
		words = []string{}
	}
	if err := writeRootFile(root, nameTokens, anon.Tokens{Format: anon.FormatVersion, Words: words}); err != nil {
		return err
	}
	for _, cr := range res.Cases {
		if cr.Case == nil {
			continue
		}
		dir := "anon-" + cr.Category
		if err := root.Mkdir(dir, 0o700); err != nil {
			return translate(stageWrite, err, "")
		}
		if err := root.Chmod(dir, 0o700); err != nil {
			return translate(stageWrite, err, "")
		}
		if err := writeRootFile(root, dir+"/case.json", cr.Case); err != nil {
			return err
		}
	}
	return nil
}

func writeRootFile(root *os.Root, name string, v any) error {
	data, err := encodeJSON(v)
	if err != nil {
		return translate(stageWrite, err, "")
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return translate(stageWrite, err, "")
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = root.Chmod(name, 0o600)
	}
	if werr != nil {
		return translate(stageWrite, werr, "")
	}
	return nil
}

func readLimited(fsys fs.FS, name string) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(f)
	return io.ReadAll(io.LimitReader(f, anon.MaxScanSize+1))
}

// scanDir runs the leak scanner over a directory through os.Root: symbolic
// links are reported and never followed. It returns the findings in a fixed
// order and the number of files seen.
func scanDir(dir string, salt []byte) ([]anon.Finding, int, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, translate(stageCheckDir, err, "")
	}
	defer closeQuietly(root)
	fsys := root.FS()
	var findings []anon.Finding
	words := map[string]bool{}
	st, err := root.Lstat(nameTokens)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		findings = append(findings, anon.Finding{File: nameTokens, Rule: anon.RuleNoDictionary})
	case err != nil:
		return nil, 0, translate(stageCheckDir, err, "")
	case st.Mode().IsRegular() && st.Size() <= anon.MaxScanSize:
		data, err := readLimited(fsys, nameTokens)
		if err != nil {
			return nil, 0, translate(stageCheck, err, "")
		}
		// A dictionary that the mapping could not have written is not
		// trusted: its words are scanned like any other.
		var tk anon.Tokens
		if json.Unmarshal(data, &tk) != nil || !anon.ValidDictionary(tk) {
			findings = append(findings, anon.Finding{File: nameTokens, Rule: anon.RuleBadDictionary})
			break
		}
		for _, w := range tk.Words {
			words[w] = true
		}
	}
	files := 0
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." || d.IsDir() {
			return nil
		}
		files++
		add := func(rule string) { findings = append(findings, anon.Finding{File: p, Rule: rule}) }
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			add(anon.RuleSymlink)
		case !d.Type().IsRegular() || !strings.HasSuffix(p, ".json"):
			add(anon.RuleUnexpectedFile)
		default:
			data, err := readLimited(fsys, p)
			if err != nil {
				return err
			}
			if len(data) > anon.MaxScanSize {
				add(anon.RuleTooLarge)
				return nil
			}
			findings = append(findings, anon.Scan(p, data, words, salt)...)
		}
		return nil
	})
	if err != nil {
		return nil, 0, translate(stageCheck, err, "")
	}
	return findings, files, nil
}

// outputNames are the file names the build can write. Any other name in the
// checked directory may be a private string, so it is printed as "<file>".
var outputNames = func() map[string]bool {
	names := map[string]bool{nameTokens: true}
	for _, c := range anon.Categories {
		names["anon-"+c.Name+"/case.json"] = true
	}
	return names
}()

func printFindings(w io.Writer, findings []anon.Finding, files int) {
	for _, f := range findings {
		name := f.File
		if !outputNames[name] {
			name = "<file>"
		}
		_, _ = fmt.Fprintf(w, "%s: %s: %s\n", name, f.Pointer, f.Rule) // nothing more to do if stdout is broken
	}
	_, _ = fmt.Fprintf(w, "check: %d findings in %d files\n", len(findings), files)
}

// runCheck is the --check mode; it returns the number of findings.
func runCheck(dir, saltPath string, stdout io.Writer) (int, error) {
	var salt []byte
	if saltPath != "" {
		s, _, err := loadSalt(saltPath, false)
		if err != nil {
			return 0, err
		}
		salt = s
	}
	findings, files, err := scanDir(dir, salt)
	if err != nil {
		return 0, err
	}
	printFindings(stdout, findings, files)
	return len(findings), nil
}

// checkPaths validates the directories and the salt file of a build before
// anything is created.
func checkPaths(in, out, saltPath string) error {
	st, err := os.Stat(in)
	if err != nil {
		return translate(stageIn, err, "")
	}
	if !st.IsDir() {
		return fail(stageIn, kindNotDir)
	}
	switch st, err := os.Lstat(out); {
	case errors.Is(err, fs.ErrNotExist):
		pst, err := os.Stat(filepath.Dir(out))
		if err != nil {
			return translate(stageOut, err, "")
		}
		if !pst.IsDir() {
			return fail(stageOut, kindNotDir)
		}
	case err != nil:
		return translate(stageOut, err, "")
	case !st.IsDir():
		return fail(stageOut, kindNotDir)
	default:
		entries, err := os.ReadDir(out)
		if err != nil {
			return translate(stageOut, err, "")
		}
		if len(entries) > 0 {
			return fail(stageOut, kindNotEmpty)
		}
	}
	realIn, err1 := realPath(in)
	realOut, err2 := realPath(out)
	realSalt, err3 := realPath(saltPath)
	if err := errors.Join(err1, err2, err3); err != nil {
		return translate(stageOut, err, "")
	}
	switch {
	// --out is missing or empty, so it cannot contain --in.
	case within(realOut, realIn):
		return fail(stageOut, kindInsideIn)
	case inGitTree(realOut):
		return fail(stageOut, kindInsideGit)
	case within(realSalt, realIn):
		return fail(stageSalt, kindInsideIn)
	case within(realSalt, realOut):
		return fail(stageSalt, kindInsideOut)
	case inGitTree(filepath.Dir(realSalt)):
		return fail(stageSalt, kindInsideGit)
	}
	return nil
}

// build is the build mode: checks, salt, material, cases, output, self-check.
func build(in, out, saltPath string, stdout, stderr io.Writer) error {
	if err := checkPaths(in, out, saltPath); err != nil {
		return err
	}
	salt, created, err := loadSalt(saltPath, true)
	if err != nil {
		return err
	}
	if created {
		_, _ = io.WriteString(stderr, "anonymize: created a new salt file\n") // nothing more to do if stderr is broken
	}
	root, err := os.OpenRoot(in)
	if err != nil {
		return translate(stageIn, err, "")
	}
	defer closeQuietly(root)
	m, which, err := readMaterial(root.FS())
	if err != nil {
		return err
	}
	mp, err := anon.NewMapper(salt)
	if err != nil {
		return fail(stageSalt, kindNotHex)
	}
	res, err := anon.Build(m, mp)
	var re *anon.RelationError
	switch {
	case errors.As(err, &re):
		return fail(stageRelations, re.Error())
	case err != nil:
		return translate(stageBuild, err, "")
	}
	if err := writeOutput(out, res); err != nil {
		return err
	}
	printAggregates(stdout, which, res)
	findings, files, err := scanDir(out, salt)
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		printFindings(stdout, findings, files)
		return fail(stageCheck, strconv.Itoa(len(findings))+" findings in the output")
	}
	return nil
}

// printAggregates prints the numbers of a build; they never hold values.
func printAggregates(w io.Writer, which string, res *anon.Result) {
	var b strings.Builder
	total := 0
	for _, n := range res.Links {
		total += n
	}
	fmt.Fprintf(&b, "inventory: %s\nlinks: pairs=%d", which, total)
	for i, l := range anon.Levels {
		fmt.Fprintf(&b, " %s=%d", l, res.Links[i])
	}
	fmt.Fprintf(&b, "\nunits: total=%d unsupported=%d too_large=%d exhausted=%d\nunsupported:",
		res.Units.Total, res.Units.Unsupported, res.Units.TooLarge, res.Units.Exhausted)
	for _, r := range anon.UnsupportedRules {
		fmt.Fprintf(&b, " %s=%d", r, res.Rules[r])
	}
	b.WriteByte('\n')
	cases := 0
	for _, cr := range res.Cases {
		if cr.Case == nil {
			fmt.Fprintf(&b, "case anon-%s: empty\n", cr.Category)
			continue
		}
		cases++
		pairs := 0
		for _, n := range cr.Levels {
			pairs += n
		}
		fmt.Fprintf(&b, "case anon-%s: records=%d pairs=%d", cr.Category, cr.Records, pairs)
		for i, l := range anon.Levels {
			if cr.Levels[i] > 0 {
				fmt.Fprintf(&b, " %s=%d", l, cr.Levels[i])
			}
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "cases: %d\n", cases)
	_, _ = io.WriteString(w, b.String()) // nothing more to do if stdout is broken
}
