package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var (
	sendThread string
	sendFlags  msgFlags
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
	Use:   "send <#channel|@user|id> [text]",
	Short: "Post a message (Markdown by default; --at schedules it)",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := channelRef(cmd.Context(), args[0])
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
		text := ""
		if len(args) == 2 {
			text = args[1]
		}
		return runMessage(cmd.Context(), id, thread, text, &sendFlags)
	},
}

func init() {
	sendCmd.Flags().StringVar(&sendThread, "thread", "", "reply in this thread (parent ts or permalink)")
	sendFlags.register(sendCmd, true)
	rootCmd.AddCommand(sendCmd)
}
