package test

import (
	"fmt"
	"strings"
)

// MatchLog looks for the key/val in the input (a log file)
func MatchLog(input string, startLine int, args ...interface{}) bool {
	size := len(args)
	if size < 2 || size%2 != 0 {
		panic("args must be even")
	}
	lines := strings.Split(input, "\n")
	if len(lines) < startLine {
		panic("log too short, lines:" + string(len(lines)))
	}
	var lookFor string
	// for each line
	found := false
	for i := startLine - 1; i < len(lines); i++ {
		// for each pair
		for j := 0; j < len(args); j++ {
			if j%2 != 0 {
				continue
			}
			key := args[j]
			val := args[j+1]
			switch val.(type) {
			case string:
				// quote it
				lookFor = fmt.Sprintf(`%s="%s"`, key, val)
				break
			case fmt.Stringer:
				// quote it
				lookFor = fmt.Sprintf(`%s="%s"`, key, val)
				break
			default:
				lookFor = fmt.Sprintf(`%s=%v`, key, val)
			}
			if pos := strings.Index(lines[i], lookFor); pos != -1 {
				found = true
			} else {
				found = false
				// short circuit
				break
			}
		}
		if found {
			break
		}
	}
	return found
}
