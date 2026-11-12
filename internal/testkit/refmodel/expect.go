package refmodel

import (
	"errors"
	"maps"
	"path"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Expected returns the world that a correct two-way sync leaves after Base
// and Now under the deletes and conflict policies given, or an error when an
// asset of Now outside Recently Deleted or a file of Now lies in an album or
// a directory that no linked file of Base shows: the model does not follow
// renames. Base must be synced: every file and every asset outside Recently
// Deleted linked, every album in a directory of its own. A file of Base gone
// from its path moved to the one new path of its size, unless another file
// gone has that size. An upload takes the name and size of its asset, an
// imported asset those of its file and the path that file gets as its UUID.
// Quarantined files are not in the world, as a scan skips them.
func (m Model) Expected(deletes domain.DeletesPolicy, conflict domain.ConflictPolicy) (World, error) {
	e := expectation{Model: m, deletes: deletes, conflict: conflict, gone: map[int64]int{},
		appeared: map[int64][]string{}, slots: map[any]slot{},
		out: World{Objects: maps.Clone(m.Now.Objects), Files: map[string]File{}}}
	for p, f := range m.Base.Files {
		if _, ok := m.Now.Files[p]; !ok {
			e.gone[f.Size]++
		}
		if o, ok := m.Base.Objects[f.Key]; ok {
			s := slot{o.Album, path.Dir(p)}
			e.slots[s.place], e.slots[s.dir] = s, s
		}
	}
	for p, f := range m.Now.Files {
		if _, ok := m.Base.Files[p]; !ok {
			e.appeared[f.Size] = append(e.appeared[f.Size], p)
		}
	}
	for k, o := range m.Now.Objects {
		if _, ok := m.Base.Objects[k]; !o.Trashed {
			s := e.in(o.Album)
			if !ok {
				e.out.Files[s.dir+"/"+o.Name] = File{Key: k, Size: o.Size}
			}
		}
	}
	for p, f := range m.Base.Files {
		if _, ok := m.Base.Objects[f.Key]; ok {
			e.link(p, f.Key)
		}
	}
	for p, f := range m.Now.Files {
		if _, ok := m.Base.Files[p]; !ok && !e.moved(f.Size) {
			e.imported(p, e.in(path.Dir(p)))
		}
	}
	if e.outside {
		return World{}, errors.New("an album or a directory of now is not one of base; want no renames")
	}
	return e.out, nil
}

// expectation is one run of Expected; slots holds the albums of base by place and by directory.
type expectation struct {
	Model
	deletes  domain.DeletesPolicy
	conflict domain.ConflictPolicy
	gone     map[int64]int
	appeared map[int64][]string
	slots    map[any]slot
	out      World
	outside  bool
}

type slot struct {
	place Album
	dir   string
}

func (e *expectation) moved(size int64) bool {
	return e.gone[size] == 1 && len(e.appeared[size]) == 1
}

// fate returns the path that holds the content of the file of base at p, or p and false.
func (e *expectation) fate(p string) (string, bool) {
	if _, ok := e.Now.Files[p]; ok {
		return p, true
	}
	if size := e.Base.Files[p].Size; e.moved(size) {
		return e.appeared[size][0], true
	}
	return p, false
}

// in returns the album of base that a place or a directory names, or marks the world outside.
func (e *expectation) in(name any) slot {
	s, ok := e.slots[name]
	e.outside = e.outside || !ok
	return s
}

// link adds what the sync makes of the link of base from the file at p to the asset k.
func (e *expectation) link(p string, k domain.AssetKey) {
	now, alive := e.fate(p)
	size := e.Now.Files[now].Size
	replaced := alive && now == p && size != e.Base.Files[p].Size
	kept := alive && now == p && !replaced
	o, ok := e.Now.Objects[k]
	gone := !ok || o.Trashed
	a := e.slots[e.Base.Objects[k].Album]
	i, r := a, e.in(path.Dir(now))
	if !gone {
		i = e.in(o.Album)
	}
	conflict := i != a && r != a && i != r
	t := r
	if r == a || conflict && e.conflict == domain.ConflictICloudWins {
		t = i
	}
	switch {
	case !alive && i != a: // R-LINK-10
		e.out.Files[i.dir+"/"+o.Name] = File{Key: k, Size: o.Size}
	case !alive && !gone && e.deletes != domain.DeletesNone: // R-LINK-09
		o.Trashed = true
		e.out.Objects[k] = o
	case kept && gone && e.deletes == domain.DeletesNone: // R-LINK-08
		e.out.Files[p] = File{Size: size}
	case !alive || kept && gone: // R-LINK-08, R-LINK-09, R-LINK-12
	case conflict && e.conflict != domain.ConflictRemoteWins && e.conflict != domain.ConflictICloudWins: // R-LINK-07
		e.out.Files[now] = File{Size: size}
	case gone || replaced: // R-LINK-06, R-LINK-11, R-LINK-14
		if ok {
			o.Trashed = true
			e.out.Objects[k] = o
		}
		e.imported(now, t)
	default: // R-LINK-04, R-LINK-05, R-LINK-07, R-LINK-13, R-LINK-14
		o.Album = t.place
		e.out.Objects[k] = o
		e.out.Files[t.dir+"/"+path.Base(now)] = File{Key: k, Size: size}
	}
}

// imported adds the asset that the sync imports from the file of now at from, placed in s.
func (e *expectation) imported(from string, s slot) {
	p, size := s.dir+"/"+path.Base(from), e.Now.Files[from].Size
	e.out.Objects[domain.AssetKey{UUID: p}] = Object{Album: s.place, Name: path.Base(from), Size: size}
	e.out.Files[p] = File{Key: domain.AssetKey{UUID: p}, Size: size}
}

// Canon returns a copy of w, the world that a sync leaves, in which every
// asset that Now lacks and a file of w links is keyed, in Objects and in that
// file, by the path of that file as its UUID, as Expected keys the assets
// that a sync imports.
func (m Model) Canon(w World) World {
	c := World{Objects: maps.Clone(w.Objects), Files: maps.Clone(w.Files)}
	for p, f := range w.Files {
		o, linked := w.Objects[f.Key]
		if _, known := m.Now.Objects[f.Key]; linked && !known {
			delete(c.Objects, f.Key)
			f.Key = domain.AssetKey{UUID: p}
			c.Objects[f.Key], c.Files[p] = o, f
		}
	}
	return c
}
