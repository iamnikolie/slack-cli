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

func TestColsKeepOrderAndMissingColumns(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, CSVCols(&buf, []byte(`[{"ts":"1","text":"a"}]`), []string{"ts", "thread_ts", "text"}))
	assert.Equal(t, "ts,thread_ts,text\n1,,a\n", buf.String())
	buf.Reset()
	require.NoError(t, ListCols(&buf, []byte(`[{"b":1,"a":2}]`), []string{"b", "a"}))
	assert.Contains(t, buf.String(), "| b | a |")
}
