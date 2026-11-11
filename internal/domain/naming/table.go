package naming

import "strings"

// The Yandex Disk client for Windows cannot store these characters. A
// look-alike keeps the name readable; it is never mapped back.
var lookalikes = strings.NewReplacer(
	"/", "∕",
	`\`, "⧵",
	":", "꞉",
	"*", "∗",
	"?", "？",
	`"`, "＂",
	"<", "‹",
	">", "›",
	"|", "ǀ",
)

// Windows drops a final dot or space, and the scan of the cloud skips names
// that start with a dot: a file made under such a name would look deleted.
const (
	dotLookalike   = "．"
	spaceLookalike = " "
)

// Windows opens the device instead of the file whatever the extension.
var reservedStems = []string{
	"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$",
	"COM0", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"COM¹", "COM²", "COM³",
	"LPT0", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	"LPT¹", "LPT²", "LPT³",
}

const (
	// The cloud refuses a longer level.
	maxNameBytes = 255
	// The Windows client is estimated, not measured, to fail on paths over
	// 1024 characters. A file in quarantine moves under this prefix and must
	// still fit. The remote root is not counted: the user sets it.
	maxPathRunes = 1024 - len(".photosync/quarantine/2006-01-02/")
)
