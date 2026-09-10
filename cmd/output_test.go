package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSONReadableAndLossless(t *testing.T) {
	raw := []byte(`{"text":"\u043f\u0440\u0438\u0432\u0435\u0442 <@U1> & https:\/\/example.com","id":9007199254740993,"decimal":1.2300,"nested":[{"text":"\u0442\u0430\u043a"}],"null":null}`)
	var buf bytes.Buffer
	require.NoError(t, writeJSON(&buf, raw))
	assert.Contains(t, buf.String(), "привет <@U1> & https://example.com")
	assert.Contains(t, buf.String(), "так")
	assert.Contains(t, buf.String(), "9007199254740993")
	assert.Contains(t, buf.String(), "1.2300")
	assert.NotContains(t, buf.String(), `\u`)
	original, err := decodeObject(raw)
	require.NoError(t, err)
	decoded, err := decodeObject(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestJSONProjectionNested(t *testing.T) {
	raw := []byte(`{"ok":true,"messages":{"matches":[{"text":"hello"}],"pagination":{"page_count":2}}}`)
	got := projectOne(raw, []string{"messages.matches", "messages.pagination.page_count", "missing"})
	assert.JSONEq(t, `{"messages.matches":[{"text":"hello"}],"messages.pagination.page_count":2}`, string(got))
}

func TestTranscriptJSONFieldsWithoutAPI(t *testing.T) {
	oldJSON, oldFields, oldStdout, oldCLI := jsonOutput, fieldsFlag, os.Stdout, cli
	t.Cleanup(func() { jsonOutput, fieldsFlag, os.Stdout, cli = oldJSON, oldFields, oldStdout, oldCLI })
	jsonOutput, fieldsFlag, cli = true, []string{"ts", "text"}, nil
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer f.Close()
	os.Stdout = f
	require.NoError(t, emitTranscript(context.Background(), []byte(`[{"ts":"1700000000.000100","text":"\u0442\u0430\u043a","blocks":[{}]}]`), "C123456", "1700000000.000100"))
	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.JSONEq(t, `[{"ts":"1700000000.000100","text":"так"}]`, string(data))
	assert.Contains(t, string(data), "так")
}

func TestProjectObjectFlat(t *testing.T) {
	m := map[string]any{"id": "C1", "name": "general", "extra": "drop"}
	out := projectObject(m, []string{"id", "name"})
	assert.Equal(t, map[string]any{"id": "C1", "name": "general"}, out)
}

func TestFieldString(t *testing.T) {
	s, err := fieldString(json.RawMessage(`{"ts":"123.456"}`), "ts")
	require.NoError(t, err)
	assert.Equal(t, "123.456", s)
}
