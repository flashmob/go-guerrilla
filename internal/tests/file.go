package tests

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TemporaryFilename returns a name for a temporary file
func TemporaryFilename(t *testing.T) string {
	f, err := os.CreateTemp("", "guerrilla-")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() {
		err := os.Remove(f.Name())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			assert.NoError(t, err)
		}
	})
	return f.Name()
}
