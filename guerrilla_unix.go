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
	return rLimit.Max, nil
}
