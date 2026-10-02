package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

var (
	sendThread   string
	sendBodyFile string
	sendIDOnly   bool
)

// readBody returns text from the positional arg, or from --body-file (use "-"
// for stdin). Exactly one source must be provided.
func readBody(arg, bodyFile string) (string, error) {
	if bodyFile != "" {
		if arg != "" {
			return "", fmt.Errorf("provide text as an argument OR --body-file, not both")
		}
		if bodyFile == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
		b, err := os.ReadFile(bodyFile)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if arg == "" {
		return "", fmt.Errorf("message text required (positional arg or --body-file)")
	}
	return arg, nil
}

var sendCmd = &cobra.Command{
	Use:   "send <#channel|id> [text]",
	Short: "Post a message (chat.postMessage)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		text := ""
		if len(args) == 2 {
			text = args[1]
		}
		body, err := readBody(text, sendBodyFile)
		if err != nil {
			return err
		}
		thread := ""
		if sendThread != "" {
			_, ts, ok := parseThreadRef(sendThread)
			if !ok {
				return errInvalidTS(sendThread)
			}
			thread = ts
		}
		raw, err := postMessage(cmd, id, body, thread)
		if err != nil {
			return err
		}
		if sendIDOnly {
			return printIDOnly(raw, "ts")
		}
		return emitObj(raw, []string{"ok", "channel", "ts"})
	},
}

// postMessage sends body to channel, optionally as a thread reply.
func postMessage(cmd *cobra.Command, channel, body, threadTS string) ([]byte, error) {
	q := url.Values{}
	q.Set("channel", channel)
	q.Set("text", body)
	if threadTS != "" {
		q.Set("thread_ts", threadTS)
	}
	return cli.Call(cmd.Context(), "chat.postMessage", q)
}

func init() {
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "reply in this thread (parent ts or permalink)")
	sendCmd.Flags().StringVar(&sendBodyFile, "body-file", "", "read text from a file ('-' for stdin)")
	sendCmd.Flags().BoolVar(&sendIDOnly, "id-only", false, "print only the new message ts")
	rootCmd.AddCommand(sendCmd)
}
