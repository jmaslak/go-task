//go:build !unix

package app

// ttyID identifies the terminal standard input is connected to. On systems
// where the terminal device cannot be identified, every terminal looks alike,
// which only makes the staleness check more eager.
func ttyID() string {
	return ""
}
