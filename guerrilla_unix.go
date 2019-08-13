// +build darwin dragonfly freebsd linux netbsd openbsd

package guerrilla

import "syscall"

// getFileLimit checks how many files we can open
func getFileLimit() (uint64, error) {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		return 0, err
	}
	//unnecessary type conversions to uint64 is needed for FreeBSD
	return uint64(rLimit.Max), nil
}
