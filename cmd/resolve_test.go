package cmd

import (
	"testing"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDir() *cache.Directory {
	return &cache.Directory{
		Channels: []cache.Channel{{ID: "C123", Name: "general"}},
		Users:    []cache.User{{ID: "U999", Name: "mako", RealName: "Mako K"}},
	}
}

func TestIsChannelID(t *testing.T) {
	assert.True(t, isChannelID("C0123ABCD"))
	assert.True(t, isChannelID("D0123ABCD"))
	assert.False(t, isChannelID("#general"))
	assert.False(t, isChannelID("general"))
}

func TestResolveChannel(t *testing.T) {
	d := testDir()
	id, err := resolveChannel(d, "C555ABCD")
	require.NoError(t, err)
	assert.Equal(t, "C555ABCD", id) // raw ID passes through

	id, err = resolveChannel(d, "#general")
	require.NoError(t, err)
	assert.Equal(t, "C123", id)

	id, err = resolveChannel(d, "general")
	require.NoError(t, err)
	assert.Equal(t, "C123", id)

	_, err = resolveChannel(d, "#nope")
	assert.ErrorIs(t, err, errNotInCache)
}

func TestResolveUser(t *testing.T) {
	d := testDir()
	id, err := resolveUser(d, "@mako")
	require.NoError(t, err)
	assert.Equal(t, "U999", id)

	id, err = resolveUser(d, "U777ABCD")
	require.NoError(t, err)
	assert.Equal(t, "U777ABCD", id)
}

func TestParseMessageRef(t *testing.T) {
	// permalink → channel + ts
	ch, ts, ok := parseMessageRef("https://acme.slack.com/archives/C0123/p1700000000123456")
	require.True(t, ok)
	assert.Equal(t, "C0123", ch)
	assert.Equal(t, "1700000000.123456", ts)

	// bare ts → ts only
	ch, ts, ok = parseMessageRef("1700000000.123456")
	require.True(t, ok)
	assert.Equal(t, "", ch)
	assert.Equal(t, "1700000000.123456", ts)

	_, _, ok = parseMessageRef("garbage")
	assert.False(t, ok)
}

func TestResolveNotFoundNamesTheRef(t *testing.T) {
	_, err := resolveChannel(&cache.Directory{}, "#nope")
	require.ErrorIs(t, err, errNotInCache)
	assert.Contains(t, err.Error(), "channel #nope")
	_, err = resolveUser(&cache.Directory{}, "@ghost")
	require.ErrorIs(t, err, errNotInCache)
	assert.Contains(t, err.Error(), "user @ghost")
}
