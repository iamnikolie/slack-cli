package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/iamnikolie/slack-cli/internal/render"
)

func fieldString(data json.RawMessage, field string) (string, error) {
	m, err := decodeObject(data)
	if err != nil {
		return "", err
	}
	v, ok := m[field]
	if !ok {
		return "", fmt.Errorf("no %q in response", field)
	}
	switch x := v.(type) {
	case nil:
		return "", nil
	case json.Number:
		return x.String(), nil
	case string:
		return x, nil
	default:
		return fmt.Sprintf("%v", x), nil
	}
}

func printIDOnly(data json.RawMessage, field string) error {
	s, err := fieldString(data, field)
	if err != nil {
		return err
	}
	fmt.Println(s)
	return nil
}

func projectObject(m map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := m[f]; ok {
			out[f] = v
			continue
		}
		var value any = m
		found := true
		for _, part := range strings.Split(f, ".") {
			nested, ok := value.(map[string]any)
			if !ok {
				found = false
				break
			}
			value, ok = nested[part]
			if !ok {
				found = false
				break
			}
		}
		if found {
			out[f] = value
		}
	}
	return out
}

func decodeArray(data json.RawMessage) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var items []map[string]any
	if err := dec.Decode(&items); err != nil {
		return nil, err
	}
	return items, nil
}

func decodeObject(data json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func projectList(data json.RawMessage, fields []string) json.RawMessage {
	if len(fields) == 0 {
		return data
	}
	items, err := decodeArray(data)
	if err != nil {
		return data
	}
	out := make([]map[string]any, len(items))
	for i, m := range items {
		out[i] = projectObject(m, fields)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return data
	}
	return b
}

func projectOne(data json.RawMessage, fields []string) json.RawMessage {
	if len(fields) == 0 {
		return data
	}
	m, err := decodeObject(data)
	if err != nil {
		return data
	}
	b, err := json.Marshal(projectObject(m, fields))
	if err != nil {
		return data
	}
	return b
}

func activeFields(defaults []string) []string {
	if len(fieldsFlag) > 0 {
		return fieldsFlag
	}
	return defaults
}

func writeRaw(data json.RawMessage) error {
	return writeJSON(os.Stdout, data)
}

// Decode Slack's escaped strings before encoding readable UTF-8. UseNumber
// preserves large integer IDs and exact decimal values in arbitrary API data.
func writeJSON(w io.Writer, data json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(value)
}

func renderTable(data json.RawMessage, defaults []string) error {
	projected := projectList(data, activeFields(defaults))
	switch outputFormat {
	case "csv":
		return render.CSV(os.Stdout, projected)
	case "tsv":
		return render.TSV(os.Stdout, projected)
	default:
		return render.List(os.Stdout, projected)
	}
}

func emitList(data json.RawMessage, defaults []string) error {
	if outputFormat == "json" || jsonOutput {
		return writeRaw(projectList(data, fieldsFlag))
	}
	return renderTable(data, defaults)
}

func emitObj(data json.RawMessage, defaults []string) error {
	if outputFormat == "json" || jsonOutput {
		return writeRaw(projectOne(data, fieldsFlag))
	}
	projected := projectOne(data, activeFields(defaults))
	switch outputFormat {
	case "csv":
		return render.CSV(os.Stdout, wrapArray(projected))
	case "tsv":
		return render.TSV(os.Stdout, wrapArray(projected))
	default:
		return render.KV(os.Stdout, projected)
	}
}

func wrapArray(obj json.RawMessage) json.RawMessage {
	b, err := json.Marshal([]json.RawMessage{obj})
	if err != nil {
		return obj
	}
	return b
}
