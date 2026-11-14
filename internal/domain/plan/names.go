package plan

import (
	"errors"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
)

// Names are the names of the cloud files of one remote in a run and of the
// directories above them: those that its scan lists and its links hold in
// quarantine, and those that the run gives. A name that Free, Quarantine or
// Render gives is taken from then on, so the names depend on the order of the
// calls, which the caller makes in an order that its input alone sets.
type Names struct {
	remote string
	keys   map[[2]string]map[string]bool
	paths  map[domain.LibraryKind]map[string]bool
}

// NewNames returns the names of remote: the path of every entry of scan, its
// whole listing with the directories and the directory .photosync, and the
// QuarantinePath of every link of remote among links. The inputs are not
// changed.
func NewNames(remote string, scan []domain.RemoteEntry, links []domain.Link) *Names {
	n := &Names{remote: remote, keys: map[[2]string]map[string]bool{}, paths: map[domain.LibraryKind]map[string]bool{}}
	for _, e := range scan {
		n.take(e.Root, e.Path)
	}
	for _, l := range links {
		if l.Remote == remote {
			n.take(l.Root, l.QuarantinePath)
		}
	}
	return n
}

// Free returns the path, relative to root, of a new file named name in dir,
// a directory as path.Dir gives it: name as naming.Free gives it against the
// export.StemKey of every name in dir.
func (n *Names) Free(root domain.LibraryKind, dir, name string) string {
	p := path.Join(dir, naming.Free(name, n.keys[[2]string{string(root), dir}]))
	n.take(root, p)
	return p
}

// Quarantine returns the path, relative to root, that the file at p takes in
// quarantine on day: QuarantinePath against every path of root.
func (n *Names) Quarantine(root domain.LibraryKind, day time.Time, p string) string {
	q := QuarantinePath(day, p, n.paths[root])
	n.take(root, q)
	return q
}

// Render returns the paths, relative to the root of asset.TargetLibrary, at
// which a run uploads the file of asset: those that pathtemplate.Render gives
// with the same arguments, each named by Free in its directory. An error that
// one asset meets, pathtemplate.ErrInvalidPath or pathtemplate.ErrMode,
// template.ErrTooManyValues or template.ErrRender, leaves asset without
// paths, and notices hold "plan.no_path" with the remote, the UUID of asset
// and the error. Any other error, such as an unknown field, comes from the
// template whatever the asset and is returned as it is.
func (n *Names) Render(t *template.Template, eventPath string, asset domain.Asset, fields template.Photo, mode domain.Mode, now time.Time) (paths []string, notices []domain.Notice, err error) {
	paths, err = pathtemplate.Render(t, eventPath, asset, fields, mode, now)
	switch {
	case errors.Is(err, pathtemplate.ErrInvalidPath), errors.Is(err, pathtemplate.ErrMode),
		errors.Is(err, template.ErrTooManyValues), errors.Is(err, template.ErrRender):
		return nil, []domain.Notice{{Code: "plan.no_path", Params: []string{n.remote, asset.Key.UUID, err.Error()}}}, nil
	case err != nil:
		return nil, nil, err
	}
	for i, p := range paths {
		paths[i] = n.Free(asset.TargetLibrary, path.Dir(p), path.Base(p))
	}
	return paths, nil, nil
}

// Doubles returns, by root and path, the names of n that naming.Doubles puts
// in a group: the Windows client of the cloud takes them for one, so a run
// changes no file among them and puts none below a directory among them.
// notices hold "plan.doubles" for each group with the remote, the root and
// the paths of the group, the personal root first.
func (n *Names) Doubles() (doubled map[domain.RemoteEntry]bool, notices []domain.Notice) {
	doubled = map[domain.RemoteEntry]bool{}
	for _, root := range []domain.LibraryKind{domain.LibraryPersonal, domain.LibraryShared} {
		for _, g := range naming.Doubles(slices.Collect(maps.Keys(n.paths[root]))) {
			notices = append(notices, domain.Notice{Code: "plan.doubles", Params: append([]string{n.remote, string(root)}, g...)})
			for _, p := range g {
				doubled[domain.RemoteEntry{Root: root, Path: p}] = true
			}
		}
	}
	return doubled, notices
}

// naming.Free compares the stems of one directory and QuarantinePath whole
// paths, so a name is kept both ways; the directories above it are names
// too, since a file put there makes them.
func (n *Names) take(root domain.LibraryKind, p string) {
	k := [2]string{string(root), path.Dir(p)}
	if n.keys[k] == nil {
		n.keys[k] = map[string]bool{}
	}
	if n.paths[root] == nil {
		n.paths[root] = map[string]bool{}
	}
	n.keys[k][export.StemKey(path.Base(p))], n.paths[root][p] = true, true
	if !n.paths[root][k[1]] {
		n.take(root, k[1])
	}
}
