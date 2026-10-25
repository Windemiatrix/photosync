package anon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
)

// Counters counts the partition mismatches of a material. They hold numbers
// only, never values.
type Counters struct {
	UnknownAsset int // a links key that is not an asset outside the trash
	UnknownFile  int // a linked path that is not a media file of the inventory
	DoubleLink   int // a path linked twice
	Download     int // download rows differ from the assets without links
	Upload       int // upload rows differ from the media files without a link
	Replace      int // a replace row without the same link
	Probable     int // a probable_match conflict without a probable link
	KeepCloud    int // a yandex_better conflict on a path without a link
	Derivation   int // derived inventory columns differ from the path
	Duplicate    int // an asset UUID or a path that occurs twice
}

// Zero reports whether no mismatch was found.
func (c Counters) Zero() bool { return c == Counters{} }

// String lists the counters.
func (c Counters) String() string {
	return fmt.Sprintf("unknown_asset=%d unknown_file=%d double_link=%d download=%d upload=%d replace=%d probable=%d keep_cloud=%d derivation=%d duplicate=%d",
		c.UnknownAsset, c.UnknownFile, c.DoubleLink, c.Download, c.Upload, c.Replace, c.Probable, c.KeepCloud, c.Derivation, c.Duplicate)
}

// SamePlan reports whether the plan of the rerun equals the reviewed plan:
// both are decoded as plain JSON values with exact numbers and compared
// deeply. Links of another computation cannot be expected pairs.
func SamePlan(reviewed, rerun []byte) (bool, error) {
	a, err := decodeAny(reviewed)
	if err != nil {
		return false, err
	}
	b, err := decodeAny(rerun)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(a, b), nil
}

func decodeAny(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errTrailing
	}
	return v, nil
}

// errTrailing reports data after the JSON value of a file.
var errTrailing = errors.New("trailing data after the JSON value")

// CheckPartition checks that the reference links, the inventory of m and the
// reviewed plan describe one partition of the records.
func CheckPartition(m *Material) Counters {
	var c Counters
	assets := map[string]bool{}
	for _, a := range m.Assets {
		if a.InTrash {
			continue
		}
		if assets[a.UUID] {
			c.Duplicate++
		}
		assets[a.UUID] = true
	}
	media := map[string]bool{}
	paths := map[string]bool{}
	for i, f := range m.Files {
		if paths[f.Path] {
			c.Duplicate++
		}
		paths[f.Path] = true
		if IsMedia(f.Kind) {
			media[f.Path] = true
		}
		if derivationDiffers(f, m.Derived[i]) {
			c.Derivation++
		}
	}
	linked := map[string]string{} // path -> level
	links := map[string]bool{}    // uuid \x00 path \x00 kind
	for _, key := range slices.Sorted(maps.Keys(m.Links)) {
		if !assets[key] {
			c.UnknownAsset++
		}
		for _, l := range m.Links[key] {
			if !media[l.Path] {
				c.UnknownFile++
			}
			if _, ok := linked[l.Path]; ok {
				c.DoubleLink++
			}
			linked[l.Path] = l.Kind
			links[key+"\x00"+l.Path+"\x00"+l.Kind] = true
		}
	}
	download := map[string]bool{}
	for _, d := range m.Plan.Download {
		download[d.UUID] = true
	}
	for _, a := range m.Assets {
		if !a.InTrash && (len(m.Links[a.UUID]) == 0) != download[a.UUID] {
			c.Download++
		}
	}
	for u := range download {
		if !assets[u] {
			c.Download++
		}
	}
	upload := map[string]bool{}
	for _, u := range m.Plan.Upload {
		upload[u.Path] = true
	}
	for _, f := range m.Files {
		if !IsMedia(f.Kind) {
			continue
		}
		_, isLinked := linked[f.Path]
		if !isLinked != upload[f.Path] {
			c.Upload++
		}
	}
	for p := range upload {
		if !media[p] {
			c.Upload++
		}
	}
	for _, r := range m.Plan.Replace {
		if !links[r.UUID+"\x00"+r.YandexPath+"\x00"+r.Match] {
			c.Replace++
		}
	}
	for _, cf := range m.Plan.Conflicts {
		if len(cf) == 0 {
			continue
		}
		switch cf[0] {
		case ConflictProbableMatch:
			if len(cf) < 3 || linked[cf[2]] != LevelProbable {
				c.Probable++
			}
		case ConflictYandexBetter:
			if len(cf) < 3 {
				c.KeepCloud++
				continue
			}
			if _, ok := linked[cf[2]]; !ok {
				c.KeepCloud++
			}
		}
	}
	return c
}

func derivationDiffers(f File, d Derived) bool {
	stem, _ := Splitext(BaseName(f.Path))
	fi := FolderInfoOf(f.Path)
	return d.Stem != stem || d.Folder != fi.Folder || d.Year != fi.Year || d.EventMonth != fi.EventMonth || f.Kind != KindOf(BaseName(f.Path))
}

// planIndex answers the plan decision of a pair.
type planIndex struct {
	replace map[string]bool
	keep    map[string]bool
}

func newPlanIndex(p Plan) planIndex {
	pi := planIndex{replace: map[string]bool{}, keep: map[string]bool{}}
	for _, r := range p.Replace {
		pi.replace[r.UUID+"\x00"+r.YandexPath] = true
	}
	for _, c := range p.Conflicts {
		if len(c) >= 3 && c[0] == ConflictYandexBetter {
			pi.keep[c[2]] = true
		}
	}
	return pi
}

// decision returns the plan decision of a pair: replace when the reviewed
// plan replaces the cloud copy with the original, keep_cloud when it keeps
// the cloud copy, "" otherwise.
func (pi planIndex) decision(uuid, path string) string {
	switch {
	case pi.replace[uuid+"\x00"+path]:
		return PlanReplaceDecision
	case pi.keep[path]:
		return PlanKeepCloudDecision
	}
	return ""
}
