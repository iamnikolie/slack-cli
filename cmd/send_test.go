package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBodyArg(t *testing.T) {
	b, err := readBody("hello", "")
	require.NoError(t, err)
	assert.Equal(t, "hello", b)
}

func TestReadBodyFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "msg.txt")
	require.NoError(t, os.WriteFile(f, []byte("from file"), 0600))
	b, err := readBody("", f)
	require.NoError(t, err)
	assert.Equal(t, "from file", b)
}

func TestReadBodyConflict(t *testing.T) {
	_, err := readBody("arg", "/tmp/x")
	assert.Error(t, err)
}

func TestReadBodyEmpty(t *testing.T) {
	_, err := readBody("", "")
	assert.Error(t, err)
}
