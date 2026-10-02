package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// JSON pretty-prints raw JSON to w.
func JSON(w io.Writer, data json.RawMessage) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("render.JSON: %w", err)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// cell formats a decoded JSON value for display. Numbers are decoded with
// UseNumber so integer IDs render as "5918904", not "5.918904e+06". Nested
// objects/arrays render as compact JSON.
func cell(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case json.Number:
		return n.String()
	case string:
		return n
	case bool:
		if n {
			return "true"
		}
		return "false"
	case map[string]any, []any:
		b, _ := json.Marshal(n)
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// tableCell formats a value for a Markdown table cell: newlines collapse to
// spaces and pipes are escaped so the row stays on one line and parses.
func tableCell(v any) string {
	s := cell(v)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// decodeArray decodes a JSON array of objects with UseNumber.
func decodeArray(data json.RawMessage) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var items []map[string]any
	if err := dec.Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// decodeObject decodes a single JSON object with UseNumber.
func decodeObject(data json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// sortedKeys returns the union of keys across items, sorted.
func sortedKeys(items []map[string]any) []string {
	keySet := map[string]bool{}
	for _, item := range items {
		for k := range item {
			keySet[k] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// List renders a JSON array as a Markdown table with sorted columns.
func List(w io.Writer, data json.RawMessage) error {
	return ListCols(w, data, nil)
}

// ListCols is List with explicit columns, in order; a column stays even when no
// row has it. nil cols means the sorted union of keys.
func ListCols(w io.Writer, data json.RawMessage, cols []string) error {
	items, err := decodeArray(data)
	if err != nil {
		return fmt.Errorf("render.List: %w", err)
	}
	if len(items) == 0 {
		fmt.Fprintln(w, "_No results._")
		return nil
	}

	keys := columns(items, cols)

	sep := make([]string, len(keys))
	for i := range sep {
		sep[i] = "---"
	}
	fmt.Fprintf(w, "| %s |\n", strings.Join(keys, " | "))
	fmt.Fprintf(w, "| %s |\n", strings.Join(sep, " | "))

	for _, item := range items {
		cells := make([]string, len(keys))
		for i, k := range keys {
			cells[i] = tableCell(item[k])
		}
		fmt.Fprintf(w, "| %s |\n", strings.Join(cells, " | "))
	}
	return nil
}

// KV renders a JSON object as Markdown key-value pairs.
func KV(w io.Writer, data json.RawMessage) error {
	m, err := decodeObject(data)
	if err != nil {
		return fmt.Errorf("render.KV: %w", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "**%s:** %s\n", k, cell(m[k]))
	}
	return nil
}

// CSV renders a JSON array as comma-separated values with a header row.
func CSV(w io.Writer, data json.RawMessage) error {
	return separatedValues(w, data, ",", nil)
}

// TSV renders a JSON array as tab-separated values with a header row.
func TSV(w io.Writer, data json.RawMessage) error {
	return separatedValues(w, data, "\t", nil)
}

// CSVCols and TSVCols take explicit columns like ListCols.
func CSVCols(w io.Writer, data json.RawMessage, cols []string) error {
	return separatedValues(w, data, ",", cols)
}

func TSVCols(w io.Writer, data json.RawMessage, cols []string) error {
	return separatedValues(w, data, "\t", cols)
}

func columns(items []map[string]any, cols []string) []string {
	if len(cols) > 0 {
		return cols
	}
	return sortedKeys(items)
}

func separatedValues(w io.Writer, data json.RawMessage, sep string, cols []string) error {
	items, err := decodeArray(data)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	if len(items) == 0 {
		return nil
	}
	keys := columns(items, cols)
	fmt.Fprintln(w, strings.Join(keys, sep))
	for _, item := range items {
		cells := make([]string, len(keys))
		for i, k := range keys {
			s := cell(item[k])
			switch sep {
			case ",":
				if strings.ContainsAny(s, ",\"\n") {
					s = `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
				}
			case "\t":
				s = strings.ReplaceAll(s, "\t", " ")
				s = strings.ReplaceAll(s, "\n", " ")
				s = strings.ReplaceAll(s, "\r", " ")
			}
			cells[i] = s
		}
		fmt.Fprintln(w, strings.Join(cells, sep))
	}
	return nil
}
