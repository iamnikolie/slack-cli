package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	uploadChannel     string
	uploadThread      string
	uploadComment     string
	uploadCommentFile string
	uploadTitle       string
	uploadName        string
)

type uploadOpts struct {
	Channel string // resolved channel ID; empty uploads without sharing
	Thread  string
	Comment string
	Title   string
	Name    string
}

type uploadedFile struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Path  string `json:"path"`
}

// uploadFiles runs Slack's external upload flow: getUploadURLExternal and a
// raw POST per file, then one completeUploadExternal that shares all of them.
func uploadFiles(ctx context.Context, paths []string, o uploadOpts) ([]uploadedFile, error) {
	if o.Name != "" && len(paths) != 1 {
		return nil, fmt.Errorf("--name applies to a single file")
	}
	if o.Title != "" && len(paths) != 1 {
		return nil, fmt.Errorf("--title applies to a single file")
	}
	if o.Thread != "" && o.Channel == "" {
		return nil, fmt.Errorf("--thread requires --channel")
	}
	if o.Comment != "" && o.Channel == "" {
		return nil, fmt.Errorf("--comment requires --channel")
	}
	// Validate every file before uploading any, so a typo does not leave
	// half-uploaded orphans behind.
	type local struct {
		path, name string
		size       int64
	}
	locals := make([]local, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file: %s", p)
		}
		if info.Size() == 0 {
			return nil, fmt.Errorf("empty file: %s (Slack rejects zero-byte uploads)", p)
		}
		name := filepath.Base(p)
		if o.Name != "" {
			name = o.Name
		}
		locals = append(locals, local{path: p, name: name, size: info.Size()})
	}

	type completeFile struct {
		ID    string `json:"id"`
		Title string `json:"title,omitempty"`
	}
	var complete []completeFile
	out := make([]uploadedFile, 0, len(locals))
	for _, l := range locals {
		raw, err := cli.Call(ctx, "files.getUploadURLExternal", url.Values{
			"filename": {l.name},
			"length":   {strconv.FormatInt(l.size, 10)},
		})
		if err != nil {
			return nil, err
		}
		var env struct {
			UploadURL string `json:"upload_url"`
			FileID    string `json:"file_id"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, err
		}
		if env.UploadURL == "" || env.FileID == "" {
			return nil, fmt.Errorf("files.getUploadURLExternal returned no upload_url/file_id")
		}
		f, err := os.Open(l.path)
		if err != nil {
			return nil, err
		}
		err = cli.Upload(ctx, env.UploadURL, f, l.size)
		f.Close()
		if err != nil {
			return nil, err
		}
		title := o.Title
		if title == "" {
			title = l.name
		}
		complete = append(complete, completeFile{ID: env.FileID, Title: title})
		abs, _ := filepath.Abs(l.path)
		out = append(out, uploadedFile{ID: env.FileID, Title: title, Name: l.name, Bytes: l.size, Path: abs})
	}

	filesJSON, err := json.Marshal(complete)
	if err != nil {
		return nil, err
	}
	q := url.Values{"files": {string(filesJSON)}}
	if o.Channel != "" {
		q.Set("channel_id", o.Channel)
	}
	if o.Thread != "" {
		q.Set("thread_ts", o.Thread)
	}
	if o.Comment != "" {
		q.Set("initial_comment", o.Comment)
	}
	if _, err := cli.Call(ctx, "files.completeUploadExternal", q); err != nil {
		return nil, err
	}
	return out, nil
}

var filesUploadCmd = &cobra.Command{
	Use:   "upload <path>...",
	Short: "Upload local files, optionally sharing to a channel/thread (requires files:write)",
	Long: "Upload one or more local files via files.getUploadURLExternal + files.completeUploadExternal.\n" +
		"Requires files:write. With --channel the files are shared in one message\n" +
		"(optionally in --thread, with --comment); without it they stay private to you.\n" +
		"All paths are validated before any upload starts.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		o := uploadOpts{Title: uploadTitle, Name: uploadName}
		if uploadChannel != "" {
			id, err := channelRef(cmd.Context(), uploadChannel)
			if err != nil {
				return err
			}
			o.Channel = id
		}
		if uploadThread != "" {
			_, ts, ok := parseMessageRef(uploadThread)
			if !ok {
				return errInvalidTS(uploadThread)
			}
			o.Thread = ts
		}
		if uploadComment != "" || uploadCommentFile != "" {
			body, err := readBody(uploadComment, uploadCommentFile)
			if err != nil {
				return err
			}
			o.Comment = body
		}
		result, err := uploadFiles(cmd.Context(), args, o)
		if err != nil {
			return err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return emitList(data, []string{"id", "name", "bytes"})
	},
}

func init() {
	filesUploadCmd.Flags().StringVarP(&uploadChannel, "channel", "c", "", "share to this channel (#name, @user or ID)")
	filesUploadCmd.Flags().StringVar(&uploadThread, "thread", "", "share as a reply in this thread (parent ts or permalink; needs --channel)")
	filesUploadCmd.Flags().StringVar(&uploadComment, "comment", "", "message text posted with the files")
	filesUploadCmd.Flags().StringVar(&uploadCommentFile, "comment-file", "", "read the comment from a file ('-' for stdin)")
	filesUploadCmd.Flags().StringVar(&uploadTitle, "title", "", "file title (single file; default: filename)")
	filesUploadCmd.Flags().StringVar(&uploadName, "name", "", "filename shown in Slack (single file; default: basename)")
	filesCmd.AddCommand(filesUploadCmd)
}
