package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

var apiFields []string

var apiCmd = &cobra.Command{
	Use:   "api <method.name>",
	Short: "Call any Slack Web API method (escape hatch)",
	Long:  "Call any Slack Web API method with form params.\n\nExample: slk --config work api conversations.info -f channel=C0123",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		for _, kv := range apiFields {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				return fmt.Errorf("bad -f %q (expected key=value)", kv)
			}
			q.Set(k, v)
		}
		raw, err := cli.Call(cmd.Context(), args[0], q)
		if err != nil {
			return err
		}
		return writeRaw(projectOne(raw, fieldsFlag))
	},
}

func init() {
	apiCmd.Flags().StringArrayVarP(&apiFields, "field", "f", nil, "form field key=value (repeatable)")
	rootCmd.AddCommand(apiCmd)
}
