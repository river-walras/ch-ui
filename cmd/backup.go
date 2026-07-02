package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/caioricciuti/ch-ui/internal/config"
)

var (
	backupConfigPath   string
	backupDatabasePath string
)

var backupCmd = &cobra.Command{
	Use:   "backup [output-file]",
	Short: "Create a consistent backup of the CH-UI database",
	Long: `Create a consistent, point-in-time snapshot of the CH-UI SQLite database.

Uses SQLite's "VACUUM INTO", which produces a clean, fully consistent copy even
while the server is running and even with WAL-mode writes in flight — unlike a
plain "cp" of the live database file.

IMPORTANT: the backup contains credentials encrypted with APP_SECRET_KEY. To
restore on another host you MUST use the same APP_SECRET_KEY, so back it up
separately (it is not stored in the database). To restore: stop the server,
replace the database file with the backup, and start the server with the same
APP_SECRET_KEY.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Load(backupConfigPath)
		src := cfg.DatabasePath
		if backupDatabasePath != "" {
			src = backupDatabasePath
		}
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("database not found at %q: %w", src, err)
		}

		dest := fmt.Sprintf("ch-ui-backup-%s.db", time.Now().Format("2006-01-02-150405"))
		if len(args) > 0 && args[0] != "" {
			dest = args[0]
		}
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("refusing to overwrite existing file %q", dest)
		}

		db, err := sql.Open("sqlite", src)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// VACUUM INTO writes a consistent snapshot to dest.
		if _, err := db.Exec("VACUUM INTO ?", dest); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}

		info, _ := os.Stat(dest)
		fmt.Printf("Backup written to %s", dest)
		if info != nil {
			fmt.Printf(" (%.1f MB)", float64(info.Size())/(1024*1024))
		}
		fmt.Println()
		fmt.Println("Remember: also back up APP_SECRET_KEY — the backup cannot be decrypted without it.")
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVarP(&backupConfigPath, "config", "c", "", "Path to config file")
	backupCmd.Flags().StringVar(&backupDatabasePath, "database-path", "", "Path to the database (overrides config)")
	rootCmd.AddCommand(backupCmd)
}
