package render

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRendersTable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, List(&buf, []byte(`[{"id":"C1","name":"general"}]`)))
	out := buf.String()
	assert.Contains(t, out, "| id | name |")
	assert.Contains(t, out, "| C1 | general |")
}

func TestListEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, List(&buf, []byte(`[]`)))
	assert.Contains(t, buf.String(), "_No results._")
}

func TestKVRendersObject(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, KV(&buf, []byte(`{"team":"T1"}`)))
	assert.Contains(t, buf.String(), "**team:** T1")
}
