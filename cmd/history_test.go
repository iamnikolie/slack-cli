package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimeBound(t *testing.T) {
	// already a ts → passthrough
	ts, err := parseTimeBound("1700000000.000100")
	require.NoError(t, err)
	assert.Equal(t, "1700000000.000100", ts)

	// epoch seconds → passthrough
	ts, err = parseTimeBound("1700000000")
	require.NoError(t, err)
	assert.Equal(t, "1700000000", ts)

	// YYYY-MM-DD → epoch seconds string
	ts, err = parseTimeBound("2023-11-14")
	require.NoError(t, err)
	assert.Equal(t, "1699920000", ts) // 2023-11-14T00:00:00Z

	_, err = parseTimeBound("not-a-date")
	assert.Error(t, err)
}
