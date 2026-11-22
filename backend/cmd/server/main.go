package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ambatumove/core/internal/admindata"
	"github.com/ambatumove/core/internal/app"
	"github.com/ambatumove/core/internal/catalog"
	"github.com/ambatumove/core/internal/config"
	"github.com/ambatumove/core/internal/gyms"
	"github.com/ambatumove/core/internal/identity"
	"github.com/ambatumove/core/internal/storage"
	"github.com/ambatumove/core/migrations"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.LogLevel)}))
	switch command {
	case "serve":
		return serve(cfg, logger)
	case "migrate":
		return migrate(cfg)
	case "healthcheck":
		return healthcheck(cfg.HTTPAddr)
	case "seed":
		application, err := app.New(context.Background(), cfg, logger)
		if err != nil {
			return err
		}
		defer application.Close()
		if cfg.SeedSyntheticGyms {
			fmt.Println("reference catalog and MET data initialized; opt-in synthetic gyms seeded")
		} else {
			fmt.Println("reference catalog, facilities, and MET data initialized; synthetic gyms disabled")
		}
		return nil
	case "backup":
		return backup(cfg, args[1:])
	case "verify-backup":
		return verifyBackup(args[1:])
	case "schema-version":
		return schemaVersion(cfg)
	case "admin":
		return admin(cfg, args[1:])
	default:
		return fmt.Errorf("unknown command %q (supported: serve, migrate, seed, backup, verify-backup, schema-version, healthcheck, admin)", command)
	}
}

func schemaVersion(cfg config.Config) error {
	db, err := storage.OpenSQLite(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.QueryRow("SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	fmt.Println(version)
	return nil
}

func backup(cfg config.Config, args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	output := flags.String("output", "", "destination path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return fmt.Errorf("backup requires --output")
	}
	path, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if _, err = os.Stat(path); err == nil {
		return fmt.Errorf("backup destination already exists")
	}
	db, err := storage.OpenSQLite(context.Background(), cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	quoted := strings.ReplaceAll(path, "'", "''")
	_, err = db.ExecContext(context.Background(), "VACUUM INTO '"+quoted+"'")
	return err
}
func verifyBackup(args []string) error {
	flags := flag.NewFlagSet("verify-backup", flag.ContinueOnError)
	path := flags.String("path", "", "backup path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("verify-backup requires --path")
	}
	db, err := storage.OpenSQLite(context.Background(), *path)
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check failed: %s", result)
	}
	var version int
	if err = db.QueryRow("SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version < 3 {
		return fmt.Errorf("backup schema version %d is incomplete", version)
	}
	latest, err := migrations.LatestVersion()
	if err != nil {
		return fmt.Errorf("determine compatible schema version: %w", err)
	}
	if version > latest {
		return fmt.Errorf("backup schema version %d is newer than this binary supports (latest %d)", version, latest)
	}
	fmt.Printf("backup verified (schema version %d)\n", version)
	return nil
}
func admin(cfg config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("admin supports gym|exercise|equipment|facility|met import|export, partner-key create|list|rotate|revoke and user create-admin")
	}
	ctx := context.Background()
	db, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = storage.ApplyMigrations(ctx, db, migrations.FS); err != nil {
		return err
	}
	if kind, ok := admindata.ParseKind(args[0]); ok {
		return adminReferenceData(ctx, db, kind, args[1:])
	}
	service, err := identity.NewService(db, cfg.JWTSigningKey, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return err
	}
	switch {
	case args[0] == "partner-key" && args[1] == "create":
		flags := flag.NewFlagSet("partner-key create", flag.ContinueOnError)
		group := flags.String("group", "", "consumer group")
		scopesRaw := flags.String("scopes", "", "comma-separated scopes")
		if err = flags.Parse(args[2:]); err != nil {
			return err
		}
		scopes := []string{}
		for _, scope := range strings.Split(*scopesRaw, ",") {
			if strings.TrimSpace(scope) != "" {
				scopes = append(scopes, strings.TrimSpace(scope))
			}
		}
		metadata, key, createErr := service.ProvisionPartnerKey(ctx, *group, scopes)
		if createErr != nil {
			return createErr
		}
		fmt.Printf("Partner key %s for %s (scopes: %s)\n%s\n", metadata.ID, metadata.GroupName, strings.Join(metadata.Scopes, ","), key)
		return nil
	case args[0] == "partner-key" && args[1] == "rotate":
		flags := flag.NewFlagSet("partner-key rotate", flag.ContinueOnError)
		id := flags.String("id", "", "partner key ID")
		if err = flags.Parse(args[2:]); err != nil {
			return err
		}
		if *id == "" {
			return fmt.Errorf("partner-key rotate requires --id")
		}
		metadata, key, rotateErr := service.RotatePartnerKey(ctx, *id)
		if rotateErr != nil {
			return rotateErr
		}
		fmt.Printf("Partner key %s for %s (scopes: %s)\n%s\n", metadata.ID, metadata.GroupName, strings.Join(metadata.Scopes, ","), key)
		return nil
	case args[0] == "partner-key" && args[1] == "list":
		keys, listErr := service.ListPartnerKeys(ctx)
		if listErr != nil {
			return listErr
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(keys)
	case args[0] == "partner-key" && args[1] == "revoke":
		flags := flag.NewFlagSet("partner-key revoke", flag.ContinueOnError)
		id := flags.String("id", "", "partner key ID")
		if err = flags.Parse(args[2:]); err != nil {
			return err
		}
		if *id == "" {
			return fmt.Errorf("partner-key revoke requires --id")
		}
		if err = service.RevokePartnerKey(ctx, *id); err != nil {
			return err
		}
		fmt.Println("partner key revoked")
		return nil
	case args[0] == "user" && args[1] == "create-admin":
		flags := flag.NewFlagSet("user create-admin", flag.ContinueOnError)
		email := flags.String("email", "", "admin email")
		passwordStdin := flags.Bool("password-stdin", false, "read admin password from stdin")
		if err = flags.Parse(args[2:]); err != nil {
			return err
		}
		if !*passwordStdin {
			return fmt.Errorf("user create-admin requires --password-stdin")
		}
		line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
		if readErr != nil && len(line) == 0 {
			return readErr
		}
		password := strings.TrimRight(line, "\r\n")
		if err = service.CreateAdmin(ctx, *email, password); err != nil {
			return err
		}
		fmt.Println("admin account created")
		return nil
	default:
		return fmt.Errorf("unsupported admin operation")
	}
}

func adminReferenceData(ctx context.Context, db *sql.DB, kind admindata.Kind, args []string) error {
	if len(args) == 0 || (args[0] != "import" && args[0] != "export") {
		return fmt.Errorf("admin %s supports import|export --file PATH", kind)
	}
	operation := args[0]
	flags := flag.NewFlagSet("admin "+string(kind)+" "+operation, flag.ContinueOnError)
	filePath := flags.String("file", "", "JSON dataset path")
	dryRun := false
	if operation == "import" {
		flags.BoolVar(&dryRun, "dry-run", false, "validate without writing changes")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if strings.TrimSpace(*filePath) == "" {
		return fmt.Errorf("admin %s %s requires --file", kind, operation)
	}
	catalogService := catalog.NewService(db)
	gymService := gyms.NewService(db)
	service := admindata.New(db, catalogService, gymService)
	if operation == "import" {
		input, err := os.Open(*filePath)
		if err != nil {
			return fmt.Errorf("open import file: %w", err)
		}
		result, importErr := service.Import(ctx, kind, input, dryRun)
		closeErr := input.Close()
		if importErr != nil {
			return importErr
		}
		if closeErr != nil {
			return closeErr
		}
		if result.DryRun {
			fmt.Printf("validated %d %s records; no changes written\n", result.Count, result.Kind)
		} else {
			fmt.Printf("imported %d %s records\n", result.Count, result.Kind)
		}
		return nil
	}
	output, err := os.OpenFile(*filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open export file: %w", err)
	}
	if err = service.Export(ctx, kind, output); err != nil {
		_ = output.Close()
		return err
	}
	if err = output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	fmt.Printf("exported %s records to %s\n", kind, *filePath)
	return nil
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func serve(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application, err := app.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           application.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	case err := <-serverErr:
		return err
	}
}

func migrate(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := storage.OpenSQLite(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	return storage.ApplyMigrations(ctx, db, migrations.FS)
}

func healthcheck(addr string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1" + addr + "/health")
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("healthcheck returned HTTP %d", resp.StatusCode)
	}
	return nil
}
