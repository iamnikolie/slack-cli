package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveLoadDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SLK_HOME", dir)

	d := &Directory{
		Channels: []Channel{{ID: "C1", Name: "general", IsMember: true}},
		Users:    []User{{ID: "U1", Name: "mako", RealName: "Mako K"}},
	}
	require.NoError(t, Save(d, "work"))

	got, err := Load("work")
	require.NoError(t, err)
	require.Len(t, got.Channels, 1)
	assert.Equal(t, "general", got.Channels[0].Name)
	assert.Equal(t, "mako", got.Users[0].Name)
	assert.False(t, got.SyncedAt.IsZero())
}

func TestLoadMissingReturnsNotExist(t *testing.T) {
	t.Setenv("SLK_HOME", t.TempDir())
	_, err := Load("work")
	assert.True(t, IsNotExist(err))
}
