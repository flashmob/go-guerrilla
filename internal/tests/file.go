package tests

import (
	"errors"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TemporaryFilename returns a name for a temporary file.
// It will be deleted after test will be finished.
func TemporaryFilename(t *testing.T) string {
	name, cleanup := TemporaryFilenameCleanup(t)
	t.Cleanup(cleanup)
	return name
}

// TemporaryFilenameCleanup returns filename and function to cleanup this file.
func TemporaryFilenameCleanup(t *testing.T) (name string, cleanup func()) {
	f, err := ioutil.TempFile("", "guerrilla-")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	cleanup = func() {
		err := os.Remove(f.Name())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			assert.NoError(t, err)
		}
	}
	return f.Name(), cleanup
}
