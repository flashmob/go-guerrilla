package tests

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// GetFreePort returns port available for use.
func GetFreePort(t *testing.T) (port int) {
	t.Helper()
	a, err := net.ResolveTCPAddr("tcp", "localhost:0")
	require.NoError(t, err)
	l, err := net.ListenTCP("tcp", a)
	require.NoError(t, err)
	require.NoError(t, l.Close())
	return l.Addr().(*net.TCPAddr).Port
}
