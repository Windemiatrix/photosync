package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
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

// A toolError is built from code constants and numbers only. Errors of other
// packages may hold paths or input values and are never printed themselves.
type toolError struct{ stage, kind string }

func (e *toolError) Error() string { return e.stage + ": " + e.kind }

func fail(stage string, err error) *toolError {
	var se *json.SyntaxError
	var ue *json.UnmarshalTypeError
	kind := "failed"
	switch {
	case errors.As(err, &se):
		kind = "syntax error at offset " + strconv.FormatInt(se.Offset, 10)
	case errors.As(err, &ue):
		// For a number, Value holds the input literal after the JSON kind.
		v, _, _ := strings.Cut(ue.Value, " ")
		kind = "wrong type (" + v + ")"
	case errors.Is(err, fs.ErrNotExist):
		kind = "not found"
	case errors.Is(err, fs.ErrPermission):
		kind = "permission denied"
	}
	return &toolError{stage, kind}
}

// closeQuietly is for a file or a root that holds no unwritten data: a close
// error loses nothing there.
func closeQuietly(c io.Closer) { _ = c.Close() }

func readCatalog(fsys fs.FS) ([]anon.Asset, error) {
	var assets []anon.Asset
	if err := decodeFile(fsys, nameCatalog, &assets); err != nil {
		return nil, fail("read catalog", err)
	}
	for i, a := range assets {
		if a.UUID == "" {
			return nil, &toolError{"read catalog", "missing field uuid in record " + strconv.Itoa(i+1)}
		}
	}
	return assets, nil
}

// Every other column of the inventory (root, created, camera, mtime, ...) is
// never decoded.
type inventoryRow struct {
	anon.File
	anon.Derived
}

func readInventory(fsys fs.FS, name string, optional bool) (inv *anon.Inventory, found bool, err error) {
	var doc struct {
		Files *[]inventoryRow `json:"files"`
	}
	err = decodeFile(fsys, name, &doc)
	switch {
	case optional && errors.Is(err, fs.ErrNotExist):
		return nil, false, nil
	case err != nil:
		return nil, false, fail("read inventory", err)
	case doc.Files == nil:
		return nil, false, &toolError{"read inventory", "missing field files"}
	}
	inv = &anon.Inventory{}
	for i, r := range *doc.Files {
		n := strconv.Itoa(i + 1)
		switch {
		case r.Path == "":
			return nil, false, &toolError{"read inventory", "missing field files.path in record " + n}
		case strings.HasPrefix(r.Path, "/") || strings.Contains(r.Path, `\`):
			return nil, false, &toolError{"read inventory", "invalid value in field files.path in record " + n}
		case r.LivePair != "" && r.LivePair != anon.LivePairCID && r.LivePair != anon.LivePairName:
			// The value is copied into case.json: only the known markers.
			return nil, false, &toolError{"read inventory", "invalid value in field files.live_pair in record " + n}
		}
		inv.Files = append(inv.Files, r.File)
		inv.Derived = append(inv.Derived, r.Derived)
	}
	return inv, true, nil
}

func readLinks(fsys fs.FS) (anon.Links, error) {
	var raw map[string][][]string
	if err := decodeFile(fsys, nameLinks, &raw); err != nil {
		return nil, fail("read links", err)
	}
	invalid := &toolError{"read links", "invalid value in field links"}
	links := anon.Links{}
	for k, ls := range raw {
		links[k] = []anon.Link{}
		for _, l := range ls {
			if len(l) != 2 {
				return nil, invalid
			}
			if _, ok := anon.LevelOf(l[1]); !ok {
				return nil, invalid
			}
			links[k] = append(links[k], anon.Link{Path: l[0], Kind: l[1]})
		}
	}
	return links, nil
}

func readMaterial(fsys fs.FS) (m *anon.Material, usedInventory string, err error) {
	reviewed, err := readInput(fsys, namePlan)
	if err != nil {
		return nil, "", fail("read plan", err)
	}
	var plan anon.Plan
	if err := json.Unmarshal(reviewed, &plan); err != nil {
		return nil, "", fail("read plan", err)
	}
	rerun, err := readInput(fsys, nameRerunPlan)
	if err != nil {
		return nil, "", fail("plan rerun", err)
	}
	same, err := anon.SamePlan(reviewed, rerun)
	if err != nil {
		// The reviewed plan has just decoded: the error is in the rerun.
		return nil, "", fail("plan rerun", err)
	}
	if !same {
		return nil, "", &toolError{"plan rerun", "plan rerun differs from the reviewed plan"}
	}
	links, err := readLinks(fsys)
	if err != nil {
		return nil, "", err
	}
	assets, err := readCatalog(fsys)
	if err != nil {
		return nil, "", err
	}
	m = &anon.Material{Assets: assets, Plan: plan, Links: links}
	var parts []string
	for _, c := range []struct {
		name, file string
		optional   bool
	}{{"current", nameInventory, false}, {"prev", namePrevInventory, true}} {
		inv, found, err := readInventory(fsys, c.file, c.optional)
		if err != nil {
			return nil, "", err
		}
		if !found {
			break
		}
		m.Inventory = *inv
		cnt := anon.CheckPartition(m)
		if cnt.Zero() {
			return m, c.name, nil
		}
		parts = append(parts, c.name+": "+cnt.String())
	}
	return nil, "", &toolError{"partition", "no inventory passes the check: " + strings.Join(parts, "; ")}
}

func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func within(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

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

func makeOut(out, realIn string) (string, error) {
	if err := os.MkdirAll(out, 0o700); err != nil {
		return "", fail("out dir", err)
	}
	real, err := resolve(out)
	if err != nil {
		return "", fail("out dir", err)
	}
	entries, err := os.ReadDir(real)
	switch {
	case err != nil:
		return "", fail("out dir", err)
	case within(real, realIn):
		return "", &toolError{"out dir", "inside --in"}
	case inGitTree(real):
		return "", &toolError{"out dir", "inside a git working tree"}
	case len(entries) > 0:
		return "", &toolError{"out dir", "not empty"}
	}
	return real, nil
}

func makeSaltDir(saltPath string) error {
	dir := filepath.Dir(saltPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail("salt", err)
	}
	real, err := resolve(dir)
	if err != nil {
		return fail("salt", err)
	}
	if inGitTree(real) {
		return &toolError{"salt", "inside a git working tree"}
	}
	return nil
}

func loadSalt(path string, create bool) (salt []byte, created bool, err error) {
	st, err := os.Stat(path)
	switch {
	case create && errors.Is(err, fs.ErrNotExist):
		salt = make([]byte, 32)
		_, _ = rand.Read(salt) // crypto/rand.Read never returns an error
		if err := writeNew(path, []byte(hex.EncodeToString(salt)+"\n")); err != nil {
			return nil, false, fail("salt", err)
		}
		return salt, true, nil
	case err != nil:
		return nil, false, fail("salt", err)
	case st.Mode().Perm()&0o077 != 0:
		return nil, false, &toolError{"salt", "permissions too open"}
	}
	real, err := resolve(path)
	if err != nil {
		return nil, false, fail("salt", err)
	}
	if inGitTree(filepath.Dir(real)) {
		return nil, false, &toolError{"salt", "inside a git working tree"}
	}
	data, err := os.ReadFile(path) //nolint:gosec // the path is the --salt-file flag of the developer running the tool
	if err != nil {
		return nil, false, fail("salt", err)
	}
	salt, err = hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(salt) != 32 {
		return nil, false, &toolError{"salt", "not 64 hex digits"}
	}
	return salt, false, nil
}

func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // a path under --out or the --salt-file flag of the developer running the tool
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

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

func writeOutputFile(path string, v any) error {
	data, err := encodeJSON(v)
	if err == nil {
		err = writeNew(path, data)
	}
	if err != nil {
		return fail("write", err)
	}
	return nil
}

func writeOutput(out string, res *anon.Result) error {
	words := res.Words
	if words == nil {
		words = []string{}
	}
	if err := writeOutputFile(filepath.Join(out, nameTokens), anon.Tokens{Format: anon.FormatVersion, Words: words}); err != nil {
		return err
	}
	for _, cr := range res.Cases {
		if cr.Case == nil {
			continue
		}
		dir := filepath.Join(out, "anon-"+cr.Category)
		if err := os.Mkdir(dir, 0o700); err != nil {
			return fail("write", err)
		}
		if err := writeOutputFile(filepath.Join(dir, "case.json"), cr.Case); err != nil {
			return err
		}
	}
	return nil
}

func scanDir(dir string, salt []byte) (findings []anon.Finding, filesSeen int, err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, fail("check dir", err)
	}
	defer closeQuietly(root)
	fsys := root.FS()
	words := map[string]bool{}
	data, err := fs.ReadFile(fsys, nameTokens)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		findings = append(findings, anon.Finding{File: nameTokens, Rule: anon.RuleNoDictionary})
	case err != nil:
		return nil, 0, fail("check", err)
	default:
		// A broken dictionary is empty; the scan of the file reports it.
		var tk anon.Tokens
		if json.Unmarshal(data, &tk) == nil {
			for _, w := range tk.Words {
				words[w] = true
			}
		}
	}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		filesSeen++
		if !d.Type().IsRegular() || !strings.HasSuffix(p, ".json") {
			findings = append(findings, anon.Finding{File: p, Rule: anon.RuleUnexpectedFile})
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		findings = append(findings, anon.Scan(p, data, words, salt)...)
		return nil
	})
	if err != nil {
		return nil, 0, fail("check", err)
	}
	return findings, filesSeen, nil
}

// A name outside outputNames may be a private string, so it is printed as
// "<file>".
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

func build(in, out, saltPath string, stdout, stderr io.Writer) error {
	root, err := os.OpenRoot(in)
	if err != nil {
		return fail("in dir", err)
	}
	defer closeQuietly(root)
	realIn, err := resolve(in)
	if err != nil {
		return fail("in dir", err)
	}
	realOut, err := makeOut(out, realIn)
	if err != nil {
		return err
	}
	if err := makeSaltDir(saltPath); err != nil {
		return err
	}
	salt, created, err := loadSalt(saltPath, true)
	if err != nil {
		return err
	}
	if created {
		_, _ = io.WriteString(stderr, "anonymize: created a new salt file\n") // nothing more to do if stderr is broken
	}
	m, which, err := readMaterial(root.FS())
	if err != nil {
		return err
	}
	mp, err := anon.NewMapper(salt)
	if err != nil {
		return fail("salt", err)
	}
	res, err := anon.Build(m, mp)
	switch {
	case errors.Is(err, anon.ErrRelations):
		return &toolError{"relations", err.Error()}
	case err != nil:
		return fail("build", err)
	}
	if err := writeOutput(realOut, res); err != nil {
		return err
	}
	printAggregates(stdout, which, res)
	findings, files, err := scanDir(realOut, salt)
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		printFindings(stdout, findings, files)
		return &toolError{"check", strconv.Itoa(len(findings)) + " findings in the output"}
	}
	return nil
}

// printAggregates prints numbers only, never values.
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
	b.WriteString("\nrelations:")
	for _, k := range slices.Sorted(maps.Keys(res.Rules)) {
		if rel, ok := strings.CutPrefix(k, "relation:"); ok {
			fmt.Fprintf(&b, " %s=%d", rel, res.Rules[k])
		}
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
