package plan

import (
	"cmp"
	"path"
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
)

// FileOps returns the operations of a run on the files of the original or the
// source role of an asset in remote r, and made, the album pairs that they
// make. decisions and placements are what DecideLayout gives for r, target is
// its target, files gives by link ID the entry of the scan where the file of
// a link lies now, as ReadScan gives it, albums are the albums of the run and
// pairs its album pairs in every remote and root; names are the names of r in
// the run, which the operations take, and t, fields and now are as
// Names.Render takes them, with the mode of r. A decision with
// ActionQuarantineFile, in the order of link ID, moves the file from where it
// lies to quarantine on day now (OpQuarantine), and its link, taking that
// place, stays quarantined_remote. A placement, in its order, goes to the
// directory that pathtemplate.EventPath gives the album AlbumID, zero for the
// day folder, with the pairs of r for the root of target.TargetLibrary, and
// made pairs an album that has none there with that directory by
// domain.OriginICloud. ActionMoveFile and ActionCopyFile take the file from
// where it lies to that directory and root, named by Names.Free (OpMove,
// OpCopy); ActionUpload puts the original of target at each path of
// Names.Render (OpPut), its notices going to notices and its error returned
// as it is; ActionQuarantineFile moves the file to quarantine, and its link
// leaves the state. A moved link takes the new place and album, a copy is a
// new link like its source, and a put makes one of the original role at the
// level uploaded with the size and hashes of the original. An operation takes
// the rule of its decision or placement and the asset of its link. The
// operations are in the order of domain.Op.Compare, made in the order of
// placements, and the inputs but names are not changed.
//
// decisions also hold what DecideDerived gives for the links of the asset in
// r. One with ActionReplaceFile on a file of the original or the source role
// that no placement moves or takes to quarantine replaces the file where it
// lies, in the order of link ID among those with ActionQuarantineFile, before
// the placements: the resource of target with the role of the file goes up
// under the name that Names.Free gives it in the directory and root of the
// file (OpPut), and the resource of each Live Photo video and RAW file that
// ComponentBeside finds beside the file among the decisions with
// ActionReplaceFile goes up at ComponentPath from that name. Each such link
// keeps its ID and album and takes its new path at the level uploaded with
// the size and hashes of its resource, and each old file first moves to
// quarantine with its link quarantined_remote (OpQuarantine); nothing is
// replaced while target lacks the resource of the file. A placement with
// ActionMoveFile of a file to replace makes that replacement in its
// directory, root and album instead of the move, one with ActionCopyFile puts
// the original there as a new link instead of the copy, and one with
// ActionQuarantineFile takes the Live Photo videos and RAW files beside the
// file among those decisions to quarantine with it, their links leaving the
// state.
func FileOps(names *Names, r domain.RemoteSpec, target domain.Asset, decisions []domain.LinkDecision, placements []domain.Placement, files map[domain.LinkID]domain.RemoteEntry, albums []domain.Album, pairs []domain.AlbumPair, t *template.Template, fields template.Photo, p domain.Policies, now time.Time) (ops []domain.Op, made []domain.AlbumPair, notices []domain.Notice, err error) {
	root := target.TargetLibrary
	add := func(kind domain.OpKind, from, to domain.Link, dst, rule string) {
		ops = append(ops, domain.Op{
			Kind: kind, Remote: r.Name, Root: from.Root, Src: from.Path, DstRoot: to.Root, Dst: dst,
			Asset: to.AssetID, Rule: rule, Links: []domain.Link{to},
		})
	}
	quarantine := func(l domain.Link, status domain.LinkStatus, rule string) {
		to := l
		to.Status, to.QuarantinePath = status, names.Quarantine(l.Root, now, l.Path)
		add(domain.OpQuarantine, l, to, to.QuarantinePath, rule)
	}
	put := func(l domain.Link, at func(name string) string, rule string) (dst string) {
		for _, res := range target.Resources {
			// The roles of a link and of its resource store the same strings.
			if res.Role == domain.ResourceRole(l.Role) {
				dst = at(res.Filename)
				add(domain.OpPut, domain.Link{}, domain.Link{
					ID: l.ID, AssetID: target.ID, Remote: r.Name, Root: l.Root, Path: dst, Role: l.Role, AlbumID: l.AlbumID,
					Size: res.Size, SHA256: res.SHA256, MD5: res.MD5, Level: domain.LevelUploaded, Status: domain.StatusActive,
				}, dst, rule)
			}
		}
		return dst
	}
	decided := slices.SortedFunc(slices.Values(decisions), func(x, y domain.LinkDecision) int { return cmp.Compare(x.Link.ID, y.Link.ID) })
	replaced := slices.DeleteFunc(slices.Clone(decided), func(d domain.LinkDecision) bool { return d.Action != domain.ActionReplaceFile })
	replace := func(main domain.LinkDecision, to domain.Link, dir string) {
		dst := put(to, func(name string) string { return names.Free(to.Root, dir, name) }, main.Rule)
		// Without the original in its place, quarantine would only take the copy away.
		if dst == "" {
			return
		}
		quarantine(main.Link, domain.StatusQuarantinedRemote, main.Rule)
		for _, d := range replaced {
			if ComponentBeside(main.Link, d.Link) {
				to.ID, to.Role = d.Link.ID, d.Link.Role
				put(to, func(string) string { return ComponentPath(dst, d.Link.Role, target) }, d.Rule)
				quarantine(d.Link, domain.StatusQuarantinedRemote, d.Rule)
			}
		}
	}
	stays := func(id domain.LinkID) bool {
		return !slices.ContainsFunc(placements, func(pl domain.Placement) bool { return pl.Link.ID == id && pl.Action != domain.ActionCopyFile })
	}
	for _, d := range decided {
		switch {
		case d.Action == domain.ActionQuarantineFile:
			l := d.Link
			l.Root, l.Path = files[l.ID].Root, files[l.ID].Path
			quarantine(l, domain.StatusQuarantinedRemote, d.Rule)
		case d.Action == domain.ActionReplaceFile && objectFile(d.Link) && stays(d.Link.ID):
			replace(d, d.Link, path.Dir(d.Link.Path))
		}
	}
	own := slices.DeleteFunc(slices.Clone(pairs), func(x domain.AlbumPair) bool { return x.Remote != r.Name || x.Root != root })
	for _, pl := range placements {
		if pl.Action == domain.ActionQuarantineFile {
			quarantine(pl.Link, "", pl.Rule)
			for _, d := range replaced {
				if ComponentBeside(pl.Link, d.Link) {
					quarantine(d.Link, "", pl.Rule)
				}
			}
			continue
		}
		var album domain.Album
		if i := slices.IndexFunc(albums, func(a domain.Album) bool { return a.ID == pl.AlbumID }); i >= 0 {
			album = albums[i]
		}
		dir := pathtemplate.EventPath(target, album, own, p)
		if album.ID != 0 && !slices.ContainsFunc(own, func(x domain.AlbumPair) bool { return x.AlbumID == album.ID }) {
			made = append(made, domain.AlbumPair{AlbumID: album.ID, Remote: r.Name, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud})
		}
		if pl.Action != domain.ActionUpload {
			kind, to := domain.OpMove, pl.Link
			if pl.Action == domain.ActionCopyFile {
				kind, to.ID = domain.OpCopy, 0
			}
			to.Root, to.AlbumID = root, pl.AlbumID
			switch i := slices.IndexFunc(replaced, func(d domain.LinkDecision) bool { return d.Link.ID == pl.Link.ID }); {
			case i >= 0 && kind == domain.OpMove:
				replace(replaced[i], to, dir)
			case i >= 0:
				put(to, func(name string) string { return names.Free(root, dir, name) }, pl.Rule)
			default:
				to.Path = names.Free(root, dir, path.Base(pl.Link.Path))
				add(kind, pl.Link, to, to.Path, pl.Rule)
			}
			continue
		}
		var paths []string
		var more []domain.Notice
		if paths, more, err = names.Render(t, dir, target, fields, r.Mode, now); err != nil {
			return nil, nil, nil, err
		}
		notices = append(notices, more...)
		for _, q := range paths {
			put(domain.Link{Root: root, Role: domain.RoleOriginal, AlbumID: pl.AlbumID}, func(string) string { return q }, pl.Rule)
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops, made, notices, nil
}
