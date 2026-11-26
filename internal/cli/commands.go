package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/the-del-mar-code-company/reeltube-cli/internal/upload"
)

// Version info - set during build
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// configKey is the context key for Config
type configKey struct{}

// GetConfig retrieves Config from command context
func GetConfig(cmd *cobra.Command) *Config {
	return cmd.Context().Value(configKey{}).(*Config)
}

// SetConfigKey returns the config key for context
func SetConfigKey() interface{} {
	return configKey{}
}

// VersionInfo returns a string containing the version, commit, and build date
func VersionInfo() string {
	return "Version: " + Version + ", Commit: " + Commit + ", Build date: " + Date
}

// NewWhoamiCmd creates the whoami command
func NewWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"me"},
		Short:   "Ping the ReelTube API to verify authentication",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := GetConfig(cmd)
			data, err := cfg.Client.Me(cmd.Context())
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			fmt.Println(data.Profile.Handle)
		},
	}
}

// NewVersionCmd creates the version command
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Reeltube CLI",
		Long:  `All software has versions. This is Reeltube's.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(VersionInfo())
		},
	}
}

// NewUploadCmd creates the upload command
func NewUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a media file to ReelTube",
		Run: func(cmd *cobra.Command, args []string) {
			filePath, _ := cmd.Flags().GetString("file")
			uploadName, _ := cmd.Flags().GetString("name")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			validation, err := upload.ValidateFile(filePath, uploadName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("File to upload: %s\n", validation.AbsPath)
			if uploadName != "" {
				fmt.Printf("Upload name override: %s\n", validation.UploadName)
			}

			if dryRun {
				fmt.Println("\n--- Dry Run Mode ---")
				fmt.Printf("File name:  %s\n", validation.FileName)
				fmt.Printf("File size:  %s (%d bytes)\n", upload.FormatBytes(validation.Size), validation.Size)
				fmt.Printf("Extension:  %s\n", validation.Extension)
				fmt.Printf("MIME type:  %s\n", validation.MIMEType)
				fmt.Printf("Upload as:  %s\n", validation.UploadName)
				fmt.Println("\nValidation passed. File is ready for upload.")
				return
			}

			// Get client from config
			cfg := GetConfig(cmd)

			// Logic to handle file upload to ReelTube
			err = upload.MultipartUpload(cmd.Context(), cfg.Client, validation.AbsPath, validation.UploadName)
			if err != nil {
				fmt.Println("Error uploading file:", err)
				os.Exit(1)
			}
			fmt.Println("File uploaded successfully")
		},
	}

	cmd.Flags().StringP("file", "f", "", "Path to the file to upload (required)")
	cmd.Flags().StringP("name", "n", "", "Optional name to override the upload")
	cmd.Flags().Bool("dry-run", false, "Validate the file without uploading")

	return cmd
}

// formatBytesShort returns a compact human-readable string for bytes
func formatBytesShort(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// NewListCmd creates the list command
func NewListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List your media uploads",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := GetConfig(cmd)
			resp, err := cfg.Client.ListMediaUploads(cmd.Context())
			if err != nil {
				fmt.Println("Error:", err)
				return
			}

			if len(resp.MediaUploads) == 0 {
				fmt.Println("No media uploads found.")
				return
			}

			// Print header
			fmt.Printf("%-36s  %-30s  %-10s  %-12s  %s\n", "ID", "FILENAME", "SIZE", "STATUS", "CREATED")
			fmt.Println(string(make([]byte, 100)))

			for _, u := range resp.MediaUploads {
				size := formatBytesShort(u.Size)
				// Truncate filename if too long
				name := u.FileName
				if len(name) > 28 {
					name = name[:25] + "..."
				}
				fmt.Printf("%-36s  %-30s  %-10s  %-12s  %s\n",
					u.ID,
					name,
					size,
					u.Status,
					u.CreatedAt,
				)
			}
		},
	}
}

// NewDeleteCmd creates the delete command
func NewDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete a media upload by ID",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			id := args[0]
			cfg := GetConfig(cmd)

			err := cfg.Client.DeleteMediaUpload(cmd.Context(), id)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			fmt.Printf("Media upload %s deleted successfully.\n", id)
		},
	}
}
