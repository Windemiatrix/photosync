package domain

// Rules maps the ID of every sync rule to one English sentence that says what
// the rule does. An ID is R-<GROUP>-<NN> and is never renumbered or reused:
// plan operations name the rule they follow by its ID, and a test that
// exercises a rule names it in a "covers:" comment.
var Rules = map[string]string{
	"R-LINK-01": "An asset new in iCloud with no file in the cloud is uploaded to the cloud.",
	"R-LINK-02": "A file new in the cloud with no asset in iCloud is imported into iCloud.",
	"R-LINK-03": "An asset and a file that are both new are matched first and linked without a transfer when they match, otherwise each follows R-LINK-01 or R-LINK-02.",
	"R-LINK-04": "An album or library change in iCloud with an unchanged cloud file moves the file to its new folder or root.",
	"R-LINK-05": "A file moved or renamed in the cloud with an unchanged asset moves the asset to the album of its new folder.",
	"R-LINK-06": "New content in a cloud file with an unchanged asset replaces the asset in iCloud with one imported from the file.",
	"R-LINK-07": "Different changes to the same property on both sides are a conflict resolved by the conflict policy.",
	"R-LINK-08": "An asset trashed or deleted in iCloud with an unchanged cloud file applies the deletes policy to the cloud.",
	"R-LINK-09": "A file deleted in the cloud with an unchanged asset applies the deletes policy to iCloud.",
	"R-LINK-10": "A change in iCloud beats a deletion in the cloud: the file is uploaded again at its new place, with a warning.",
	"R-LINK-11": "A change in the cloud beats a deletion in iCloud: the file is imported as a new asset, with a warning.",
	"R-LINK-12": "A link deleted on both sides is cleared from the state.",
	"R-LINK-13": "The same change on both sides is not a conflict: the base takes it without operations.",
	"R-LINK-14": "Changes to different properties on the two sides, such as a new album in iCloud and new content in the cloud, are both applied.",

	"R-OBJECT-01": "An asset counts as deleted from a remote only when all its active links with the original role there are gone.",
	"R-OBJECT-02": "When one of several links of an asset disappears, the asset leaves the album of that folder and is not deleted.",
	"R-OBJECT-03": "An asset added to an event album that has a directory pair is copied into that directory as a new link.",
	"R-OBJECT-04": "An asset removed from an event album while it keeps other links has the copy in that album's directory moved to quarantine, whatever the deletes policy.",
	"R-OBJECT-05": "A removal from one event album together with an addition to another moves the file instead of copying and quarantining it.",
	"R-OBJECT-06": "An asset removed from all its event albums has its last file moved to its next primary placement.",
	"R-OBJECT-07": "An asset that moves between the shared and the personal library moves all its files to the root of the other library.",
	"R-OBJECT-08": "A move found by content among duplicate files pairs the links of one asset first, then one to one in a fixed order, and an ambiguous one becomes a deletion and a new file with a notice.",

	"R-COMPONENT-01": "A Live Photo video or RAW file deleted while its main file stays is restored from iCloud, with a warning.",
	"R-COMPONENT-02": "A Live Photo video or RAW file moved away from its main file is moved back beside it, with a warning.",
	"R-COMPONENT-03": "New content in a Live Photo video or RAW file replaces the whole asset with one built from all files of the group.",
	"R-COMPONENT-04": "Components and sidecars that share the stem of a main file follow it when it is moved or deleted.",
	"R-COMPONENT-05": "A sidecar changed, moved or deleted on its own does not affect the asset.",

	"R-MODE-01": "An upload-only remote runs only actions toward the cloud, takes the cloud side into the base as it is, only reports new cloud files and detaches the link of a file the user deleted.",
	"R-MODE-02": "A download-only remote runs only actions toward iCloud, takes the iCloud side into the base as it is, does not upload new assets and detaches the link of an asset deleted in iCloud.",
	"R-MODE-03": "A one-way remote has no conflicts, because the side it does not sync always counts as unchanged.",
	"R-MODE-04": "With albums_for_personal set to false, folder changes under the personal root leave iCloud albums as they are and files imported from it join no album.",

	"R-STATUS-01": "With deletes set to quarantine, a cloud file to delete moves to .photosync/quarantine/<date>/<path> under its root and the link becomes quarantined_remote.",
	"R-STATUS-02": "With deletes set to quarantine, an asset to delete goes to Recently Deleted in Photos and its link stays quarantined_icloud while it is there.",
	"R-STATUS-03": "A quarantined link takes no part in the rules for new assets and files, so an asset in Recently Deleted is not uploaded and a quarantined file is not imported.",
	"R-STATUS-04": "An asset or file that the user brings back from quarantine is a change that makes its link active again.",
	"R-STATUS-05": "With deletes set to none, a deletion does not spread and the link stays detached until the user relinks it.",
	"R-STATUS-06": "With deletes set to propagate, a deletion spreads to the other side and the link is removed at once, so a restored asset comes back as a new one.",
	"R-STATUS-07": "A conflict left open by the ask or skip policy is kept across runs and closes by itself once one side returns to the base.",
	"R-STATUS-08": "A replacement keeps the old asset or file in quarantine whatever the deletes policy is.",
	"R-STATUS-09": "A file kept in cloud quarantine longer than quarantine_keep_days is deleted for good.",

	"R-LAYOUT-01": "A top-level album folder named with four digits is a year folder, an album anywhere inside it is an event and every other album is thematic.",
	"R-LAYOUT-02": "The primary placement of an asset is an event album, else a thematic album, else the day folder of its capture date.",
	"R-LAYOUT-03": "Of several thematic albums the primary one is the album the asset joined first, or the first by full path when that is unknown.",
	"R-LAYOUT-04": "At the first placement of an asset in several event albums the event closest to its capture date is primary, and the multi_album policy applies once.",
	"R-LAYOUT-05": "An asset with no album goes to the day folder of its local capture date, or at its first placement into the only event folder of that day.",
	"R-LAYOUT-06": "A new album pair maps the album path to a directory and back: the year folder, MM/DD Title as MM-DD - Title, a bare MM/DD with the untitled event name, nested parts as levels and thematic albums under the albums root.",
	"R-LAYOUT-07": "The direction of an album pair is fixed by the side where the pair first appeared, so a top-level cloud folder and its album never re-create each other.",
	"R-LAYOUT-08": "Copies of one file in several cloud folders are several links of one asset in several albums, not a conflict.",
	"R-LAYOUT-09": "Irregular folders have fixed meanings: a file in the root or a year folder stays there without an album, Year/Title is an event dated by its album start or its earliest asset, Year/MM and MM - Title date to the 1st and a subfolder of a day folder is an untitled event part.",
	"R-LAYOUT-10": "At the first sync an album pairs with an unpaired directory of the same name, else with the one that holds more than half of its matched files, else it gets a new directory.",
}
