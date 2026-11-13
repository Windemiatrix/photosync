package plan

import "github.com/Windemiatrix/photosync/internal/domain"

// ModeAllows reports whether a remote in mode m runs action a. ActionNone,
// ActionLink, ActionForget and ActionUpdateBase have no operation and run in
// every mode. ActionUpload, ActionMoveFile, ActionCopyFile, ActionReplaceFile,
// ActionDeleteFile and ActionQuarantineFile change the cloud and run unless m
// is download-only; ActionImport, ActionMoveAsset, ActionReplaceAsset and
// ActionDeleteAsset change iCloud and run unless m is upload-only. Any other
// action, which changes both sides or is unknown here, runs only two-way.
func ModeAllows(m domain.Mode, a domain.LinkAction) bool {
	switch a {
	case domain.ActionNone, domain.ActionLink, domain.ActionForget, domain.ActionUpdateBase:
		return true
	case domain.ActionUpload, domain.ActionMoveFile, domain.ActionCopyFile, domain.ActionReplaceFile, domain.ActionDeleteFile, domain.ActionQuarantineFile:
		return m != domain.ModeDownloadOnly
	case domain.ActionImport, domain.ActionMoveAsset, domain.ActionReplaceAsset, domain.ActionDeleteAsset:
		return m != domain.ModeUploadOnly
	}
	return m == domain.ModeTwoWay
}

// ModeDecide returns the decision on a link of a remote in mode m, where
// DecideLink(icloud, remote, same) is the decision of a two-way remote:
// icloud and remote are the changes of the two sides as ICloudSide and
// RemoteSide give them, with a file found by content elsewhere as moved. A
// one-way remote does not sync one side, the cloud for upload-only and
// iCloud for download-only. A move of that side, and for upload-only new
// content in the file, counts as no change unless both sides moved to one
// place, and the base takes it with ActionUpdateBase when nothing else is
// done. A deletion of that side alone, and a new file or asset on it, give
// ActionNone; StatusDeletes leaves the link of such a deletion detached.
// Those decisions take the rule R-MODE-01 for upload-only or R-MODE-02 for
// download-only. ICloud and Remote of the result are icloud and remote, and
// the result is false where DecideLink's is.
func ModeDecide(m domain.Mode, icloud, remote domain.ChangeKind, same bool) (domain.LinkDecision, bool) {
	i, r, unsynced, rule := icloud, remote, remote, ""
	switch m {
	case domain.ModeUploadOnly:
		rule = "R-MODE-01"
		if r == domain.ChangeReplaced || r == domain.ChangeMoved && (!same || i != domain.ChangeMoved) {
			r = domain.ChangeUnchanged
		}
	case domain.ModeDownloadOnly:
		rule, unsynced = "R-MODE-02", icloud
		if i == domain.ChangeMoved && (!same || r != domain.ChangeMoved) {
			i = domain.ChangeUnchanged
		}
	}
	d, ok := DecideLink(i, r, same)
	d.ICloud, d.Remote = icloud, remote
	switch {
	case rule != "" && unsynced == domain.ChangeDeleted && icloud != remote, !ModeAllows(m, d.Action):
		d.Action, d.Rule = domain.ActionNone, rule
	case d.Action == domain.ActionNone && (i != icloud || r != remote):
		d.Action, d.Rule = domain.ActionUpdateBase, rule
	}
	return d, ok
}

// ModeKeepAlbums returns ModeDecide(m, icloud, remote, same) for a link
// under the personal root while albums_for_personal is false, the file of
// which, if the user moved it, is still under that root: a move of the file
// changes no album there, so it counts as no change unless both sides moved
// to one place, and when ModeDecide then does nothing and names no rule, the
// base takes the move with ActionUpdateBase and the rule R-MODE-04. Remote of
// the result is remote.
func ModeKeepAlbums(m domain.Mode, icloud, remote domain.ChangeKind, same bool) (domain.LinkDecision, bool) {
	if remote != domain.ChangeMoved || same && icloud == domain.ChangeMoved {
		return ModeDecide(m, icloud, remote, same)
	}
	d, ok := ModeDecide(m, icloud, domain.ChangeUnchanged, same)
	d.Remote = remote
	if d.Action == domain.ActionNone && d.Rule == "" {
		d.Action, d.Rule = domain.ActionUpdateBase, "R-MODE-04"
	}
	return d, ok
}
