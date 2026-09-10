package cmd

import (
	"testing"

	"github.com/iamnikolie/slack-cli/internal/cache"
	"github.com/stretchr/testify/assert"
)

func TestFilterChannels(t *testing.T) {
	in := []cache.Channel{
		{ID: "C1", Name: "general", IsPrivate: false},
		{ID: "C2", Name: "secret-ops", IsPrivate: true},
		{ID: "C3", Name: "general-chat", IsPrivate: false},
		{ID: "D1", Name: "mako", IsIM: true, User: "U1"},
		{ID: "G1", Name: "mpdm-a--b--c", IsMPIM: true, IsPrivate: true},
	}
	// no filter → everything
	assert.Len(t, filterChannels(in, "", "", 0), 5)
	// substring
	assert.Len(t, filterChannels(in, "", "general", 0), 2)
	// public only (im/mpim/private excluded)
	got := filterChannels(in, "public", "", 0)
	assert.Len(t, got, 2)
	// private only (mpim is its own kind, not private)
	got = filterChannels(in, "private", "", 0)
	assert.Len(t, got, 1)
	assert.Equal(t, "C2", got[0].ID)
	// im only
	got = filterChannels(in, "im", "", 0)
	assert.Len(t, got, 1)
	assert.Equal(t, "D1", got[0].ID)
	// mpim only
	got = filterChannels(in, "mpim", "", 0)
	assert.Len(t, got, 1)
	assert.Equal(t, "G1", got[0].ID)
	// combined types
	assert.Len(t, filterChannels(in, "im,mpim", "", 0), 2)
	// limit
	assert.Len(t, filterChannels(in, "", "", 1), 1)
}

func TestChannelKind(t *testing.T) {
	assert.Equal(t, "public", channelKind(cache.Channel{}))
	assert.Equal(t, "private", channelKind(cache.Channel{IsPrivate: true}))
	assert.Equal(t, "mpim", channelKind(cache.Channel{IsMPIM: true, IsPrivate: true}))
	assert.Equal(t, "im", channelKind(cache.Channel{IsIM: true}))
}
