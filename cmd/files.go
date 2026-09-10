package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

var reFileID = regexp.MustCompile(`^F[A-Z0-9]{6,}$`)
var fileOutput string

// fileRef accepts file IDs, Slack file permalinks, and Slack private URLs.
// A message permalink is ambiguous: a message may contain multiple attachments.
func fileRef(ref string) (string, error) {
	if reFileID.MatchString(ref) {
		return ref, nil
	}
	u, err := url.Parse(ref)
	if err == nil && u.Scheme == "https" && u.User == nil {
		host := strings.ToLower(u.Hostname())
		if host == "slack.com" || strings.HasSuffix(host, ".slack.com") || host == "slack-files.com" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			for _, part := range parts {
				if reFileID.MatchString(part) {
					return part, nil
				}
			}
			if len(parts) > 0 && (host == "slack-files.com" || (host == "files.slack.com" && parts[0] == "files-pri")) {
				for _, part := range parts {
					for _, segment := range strings.Split(part, "-") {
						if reFileID.MatchString(segment) {
							return segment, nil
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("expected a file ID (F...) or Slack file permalink; for a message link, read its thread with --json --fields ts,text,files and choose a file ID")
}

type slackFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MIME        string `json:"mimetype"`
	Size        int64  `json:"size"`
	URL         string `json:"url_private"`
	DownloadURL string `json:"url_private_download"`
	Permalink   string `json:"permalink"`
	IsExternal  bool   `json:"is_external"`
}

type downloadedFile struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	MIME      string `json:"mimetype"`
	Permalink string `json:"permalink"`
}

func downloadFile(ctx context.Context, ref, output string) (*downloadedFile, error) {
	id, err := fileRef(ref)
	if err != nil {
		return nil, err
	}
	raw, err := cli.Call(ctx, "files.info", url.Values{"file": {id}})
	if err != nil {
		return nil, err
	}
	var env struct {
		File slackFile `json:"file"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	f := env.File
	if f.ID != id {
		return nil, fmt.Errorf("files.info returned a missing or mismatched file ID")
	}
	if f.IsExternal {
		return nil, fmt.Errorf("file %s is hosted by an external provider; download it through that provider's authorized integration", id)
	}
	downloadURL := f.DownloadURL
	if downloadURL == "" {
		downloadURL = f.URL
	}
	if downloadURL == "" {
		return nil, fmt.Errorf("file %s has no private download URL; it may be restricted, deleted, or a non-downloadable Slack document", id)
	}
	name := safeFileName(id, f.Name)
	if output == "" {
		output = name
	} else if info, err := os.Stat(output); err == nil && info.IsDir() {
		output = filepath.Join(output, name)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(output); err == nil {
		return nil, fmt.Errorf("output already exists: %s; choose another --output path", output)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Download to a private temporary file, then publish without replacing any
	// existing path (including symlinks). Failed downloads leave no final file.
	tmp, err := os.CreateTemp(filepath.Dir(output), ".slk-download-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	n, err := cli.Download(ctx, downloadURL, f.MIME, tmp)
	if err != nil {
		return nil, err
	}
	if f.Size > 0 && n != f.Size {
		return nil, fmt.Errorf("incomplete file: received %d bytes, files.info reported %d", n, f.Size)
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Link(tmp.Name(), output); err != nil {
		return nil, fmt.Errorf("save download without overwriting %s: %w", output, err)
	}
	return &downloadedFile{ID: id, Path: output, Bytes: n, MIME: f.MIME, Permalink: f.Permalink}, nil
}

func safeFileName(id, name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return id
	}
	return id + "-" + name
}

var filesCmd = &cobra.Command{Use: "files", Short: "Work with Slack file attachments"}
var filesDownloadCmd = &cobra.Command{
	Use:   "download <file-id|permalink>",
	Short: "Download a Slack attachment using the profile token (requires files:read)",
	Long:  "Download a Slack-hosted file by ID or file permalink. Requires files:read.\nSaves to <file-id>-<name> in the current directory unless --output is set.\nExisting files are never overwritten. Failed downloads leave no final file.\nFor a message link, read its thread with --json --fields ts,text,files first.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := downloadFile(cmd.Context(), args[0], fileOutput)
		if err != nil {
			return err
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return emitObj(data, []string{"id", "path", "bytes", "mimetype"})
	},
}

func init() {
	filesDownloadCmd.Flags().StringVarP(&fileOutput, "output", "o", "", "destination file or existing directory (no overwrite)")
	filesCmd.AddCommand(filesDownloadCmd)
	rootCmd.AddCommand(filesCmd)
}
