package plan

import (
	"cmp"
	"slices"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// RunOps returns the operations of a run that come from the scans of its
// remotes rather than from an asset. read and added are as ReadScans gives
// them, albums are the albums of the run, those of the snapshot reconciled
// with the state and added, and names give, by the name of each of remotes,
// its names in the run, which the operations take. Each album or folder of
// added comes to iCloud by an OpAlbumCreate under R-LAYOUT-06, and each album
// of the Renamed of a remote takes its title by an OpAlbumRename with the
// title as Dst under R-LAYOUT-12, or under R-LAYOUT-14 for a folder of
// albums, and each of its Refiled its folder by an OpAlbumMove with the folder
// as ParentID under R-LAYOUT-13. Each NewFile with ActionImport, the remotes
// taken in the order of their names, gives an OpImport of its Main from where
// it lies under the rule of its decision, into its AlbumID while albums hold
// that album outside Recently Deleted and else into none, but for one whose
// Main has, by the hash of kind Capabilities.ContentHash of its remote, the
// content of the Main of one imported before, an empty hash matching none: its
// links, made as below, follow those of that import when it is of another
// remote and into the same album, and else it waits for a later run. The links of an import are new and of no asset yet:
// the Main with the role of the file, then the Live Photo video and the RAW
// file, if any, each where its entry lies, with the size, hashes, ETag and
// MTime of the entry, the level imported and the album of the import. When
// p.EditedFiles is quarantine, each of the Edited of a remote where
// ModeAllows ActionQuarantineFile moves without a link under R-STATUS-10, in
// the order of Root, then Path, to the path that Names.Quarantine gives it on
// day now. The operations are in the order of domain.Op.Compare, and the
// inputs but names are not changed.
func RunOps(read map[string]RemoteScan, added, albums []domain.Album, names map[string]*Names, remotes []domain.RemoteSpec, p domain.Policies, now time.Time) []domain.Op {
	var ops []domain.Op
	for _, a := range added {
		ops = append(ops, domain.Op{Kind: domain.OpAlbumCreate, AlbumID: a.ID, Rule: "R-LAYOUT-06"})
	}
	seen := map[string]int{}
	for _, r := range slices.SortedFunc(slices.Values(remotes), func(x, y domain.RemoteSpec) int { return cmp.Compare(x.Name, y.Name) }) {
		for _, a := range read[r.Name].Renamed {
			rule := "R-LAYOUT-12"
			if a.Kind == domain.KindFolder {
				rule = "R-LAYOUT-14"
			}
			ops = append(ops, domain.Op{Kind: domain.OpAlbumRename, AlbumID: a.ID, Dst: a.Title, Rule: rule})
		}
		for _, a := range read[r.Name].Refiled {
			ops = append(ops, domain.Op{Kind: domain.OpAlbumMove, AlbumID: a.ID, ParentID: a.ParentID, Rule: "R-LAYOUT-13"})
		}
		for _, f := range read[r.Name].NewFiles {
			if f.Decision.Action != domain.ActionImport {
				continue
			}
			album := f.AlbumID
			if !slices.ContainsFunc(albums, func(a domain.Album) bool { return a.ID == album && !a.InTrash }) {
				album = 0
			}
			link := func(e domain.RemoteEntry, role domain.LinkRole) domain.Link {
				return domain.Link{
					Remote: r.Name, Root: e.Root, Path: e.Path, Role: role, AlbumID: album, Size: e.Size,
					SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, MTime: e.MTime, Level: domain.LevelImported, Status: domain.StatusActive,
				}
			}
			links := []domain.Link{link(f.File.Main, f.File.Role)}
			if f.File.Live != nil {
				links = append(links, link(*f.File.Live, domain.RoleLiveVideo))
			}
			if f.File.RAW != nil {
				links = append(links, link(*f.File.RAW, domain.RoleRAWPair))
			}
			k := moveKey(f.File.Main.MD5, f.File.Main.SHA256, r.Capabilities.ContentHash)
			if i, ok := seen[k]; k != "" && ok {
				// A file left to wait has no link, so the next run would take it for a
				// new file of the user, whatever the user did to the asset meanwhile.
				if ops[i].Remote != r.Name && ops[i].AlbumID == album {
					ops[i].Links = append(ops[i].Links, links...)
				}
				continue
			}
			seen[k] = len(ops)
			ops = append(ops, domain.Op{
				Kind: domain.OpImport, Remote: r.Name, Root: f.File.Main.Root, Src: f.File.Main.Path, AlbumID: album,
				Rule: f.Decision.Rule, Links: links,
			})
		}
		if p.EditedFiles != domain.EditedQuarantine || !ModeAllows(r.Mode, domain.ActionQuarantineFile) {
			continue
		}
		for _, e := range slices.SortedFunc(slices.Values(read[r.Name].Edited), func(x, y domain.RemoteEntry) int {
			return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
		}) {
			ops = append(ops, domain.Op{
				Kind: domain.OpQuarantine, Remote: r.Name, Root: e.Root, Src: e.Path, DstRoot: e.Root,
				Dst: names[r.Name].Quarantine(e.Root, now, e.Path), Rule: "R-STATUS-10",
			})
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops
}
