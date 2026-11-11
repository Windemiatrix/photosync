package domain

// Notice is a warning or a remark that a plan reports to the user, such as a
// probable match or two cloud paths that differ only in case. Code is stable
// and never translated, such as "match.probable". Params are the values that
// the message of the notice shows, in their order; a number is in decimal.
type Notice struct {
	Code   string
	Params []string
}
