// +build !darwin
// +build !dragonfly
// +build !freebsd
// +build !linux
// +build !netbsd
// +build !openbsd

package guerrilla

import "errors"

// getFileLimit checks how many files we can open
// Don't know how to get that info (yet?), so returns false information & error
func getFileLimit() (uint64, error) {
	return 1000000, errors.New("syscall.RLIMIT_NOFILE not supported on your OS/platform")
}
