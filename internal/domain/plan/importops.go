package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// ImportOps returns the imports that bring the files of the links of an
// asset to iCloud as new assets: a file changed in the cloud while its asset
// is gone from iCloud or the run deletes it (R-LINK-11), or one that the user
// brought back from quarantine while its asset is not back (R-LINK-02). decisions are as
// DecideLayout gives them and links as it takes them, files gives by link ID
// the entry of the scan where the file of a link lies now, as ReadScan gives
// it, and albums are the albums of the run. Each decision with ActionImport
// whose link is in links with the original or the source role gives an
// OpImport of the file of that link from where it lies, under the rule of the
// decision, into the album of that link while albums hold it outside
// Recently Deleted and the file lies outside the personal root or
// albumsForPersonal holds, else into none. Its links are that link, then each
// Live Photo video and RAW file that ComponentBeside finds beside it in
// links, in the order of ID, each with its ID, a zero AssetID, the size,
// hashes, ETag and MTime of its entry, the level imported and the album of
// the import. The operations are in the order of domain.Op.Compare, and the
// inputs are not changed.
func ImportOps(decisions map[string][]domain.LinkDecision, links []domain.Link, files map[domain.LinkID]domain.RemoteEntry, albums []domain.Album, albumsForPersonal bool) []domain.Op {
	links = slices.SortedFunc(slices.Values(links), func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) })
	now := map[domain.LinkID]domain.Link{}
	for _, l := range links {
		now[l.ID] = l
	}
	var ops []domain.Op
	for _, ds := range decisions {
		for _, d := range ds {
			main := now[d.Link.ID]
			if d.Action != domain.ActionImport || main.Role != domain.RoleOriginal && main.Role != domain.RoleSource {
				continue
			}
			album := main.AlbumID
			if !slices.ContainsFunc(albums, func(a domain.Album) bool { return a.ID == album && !a.InTrash }) ||
				main.Root == domain.LibraryPersonal && !albumsForPersonal {
				album = 0
			}
			imported := func(l domain.Link) domain.Link {
				e := files[l.ID]
				l.AssetID, l.AlbumID, l.Level = 0, album, domain.LevelImported
				l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
				return l
			}
			o := domain.Op{
				Kind: domain.OpImport, Remote: main.Remote, Root: main.Root, Src: main.Path, AlbumID: album, Rule: d.Rule,
				Links: []domain.Link{imported(main)},
			}
			for _, l := range links {
				if ComponentBeside(main, l) {
					o.Links = append(o.Links, imported(l))
				}
			}
			ops = append(ops, o)
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops
}
