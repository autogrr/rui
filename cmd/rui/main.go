// Copyright (c) 2025, s0up and the autobrr contributors.
// Copyright (c) 2026, the rui contributors.
// SPDX-License-Identifier: AGPL-1.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alexedwards/scs/v2"
	qbt "github.com/autogrr/go-qbittorrent"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/autogrr/rui/internal/api"
	"github.com/autogrr/rui/internal/auth"
	"github.com/autogrr/rui/internal/backups"
	"github.com/autogrr/rui/internal/buildinfo"
	"github.com/autogrr/rui/internal/config"
	"github.com/autogrr/rui/internal/database"
	"github.com/autogrr/rui/internal/domain"
	"github.com/autogrr/rui/internal/metrics"
	"github.com/autogrr/rui/internal/models"
	"github.com/autogrr/rui/internal/qbittorrent"
	"github.com/autogrr/rui/internal/services/arr"
	"github.com/autogrr/rui/internal/services/automations"
	"github.com/autogrr/rui/internal/services/crossseed"
	"github.com/autogrr/rui/internal/services/dirscan"
	"github.com/autogrr/rui/internal/services/externalprograms"
	"github.com/autogrr/rui/internal/services/filesmanager"
	"github.com/autogrr/rui/internal/services/intake"
	"github.com/autogrr/rui/internal/services/jackett"
	"github.com/autogrr/rui/internal/services/library"
	"github.com/autogrr/rui/internal/services/notifications"
	"github.com/autogrr/rui/internal/services/orphanscan"
	"github.com/autogrr/rui/internal/services/reannounce"
	"github.com/autogrr/rui/internal/services/trackericons"
	"github.com/autogrr/rui/internal/update"
	"github.com/autogrr/rui/pkg/sqlite3store"
)

func main() {
	config.InitDefaultLogger(buildinfo.Version)

	var rootCmd = &cobra.Command{
		Use:   "rui",
		Short: "A self-hosted qBittorrent WebUI alternative",
		Long: `rui - A modern, self-hosted web interface for managing
multiple qBittorrent instances with support for 10k+ torrents.`,
	}

	rootCmd.Version = buildinfo.Version

	rootCmd.AddCommand(RunServeCommand())
	rootCmd.AddCommand(RunVersionCommand(buildinfo.Version))
	rootCmd.AddCommand(RunGenerateConfigCommand())
	rootCmd.AddCommand(RunCreateUserCommand())
	rootCmd.AddCommand(RunChangePasswordCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func RunServeCommand() *cobra.Command {
	var (
		configDir string
		dataDir   string
		logPath   string
		pprofFlag bool
	)

	var command = &cobra.Command{
		Use:   "serve",
		Short: "Start the server",
	}

	command.Flags().StringVar(&configDir, "config-dir", "", "config directory path (default is OS-specific: ~/.config/rui/ or %APPDATA%\\rui\\). For backward compatibility, can also be a direct path to a .toml file")
	command.Flags().StringVar(&dataDir, "data-dir", "", "data directory for database and other files (default is next to config file)")
	command.Flags().StringVar(&logPath, "log-path", "", "log file path (default is stdout)")
	command.Flags().BoolVar(&pprofFlag, "pprof", false, "enable pprof server on :6060")

	command.Run = func(cmd *cobra.Command, args []string) {
		app := NewApplication(configDir, dataDir, logPath, pprofFlag)
		app.runServer()
	}

	return command
}

func RunVersionCommand(version string) *cobra.Command {
	var command = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of rui",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}

	return command
}

func RunGenerateConfigCommand() *cobra.Command {
	var configDir string

	command := &cobra.Command{
		Use:   "generate-config",
		Short: "Generate a default configuration file",
		Long: `Generate a default configuration file without starting the server.

If no --config-dir is specified, uses the OS-specific default location:
- Linux/macOS: ~/.config/rui/config.toml
- Windows: %APPDATA%\rui\config.toml

You can specify either a directory path or a direct file path:
- Directory: rui generate-config --config-dir /path/to/config/
- File: rui generate-config --config-dir /path/to/myconfig.toml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var configPath string
			if configDir != "" {
				if strings.HasSuffix(strings.ToLower(configDir), ".toml") {
					configPath = configDir
				} else if info, err := os.Stat(configDir); err == nil && !info.IsDir() {
					configPath = configDir
				} else {
					configPath = filepath.Join(configDir, "config.toml")
				}
			} else {
				defaultDir := config.GetDefaultConfigDir()
				configPath = filepath.Join(defaultDir, "config.toml")
			}

			if _, err := os.Stat(configPath); err == nil {
				cmd.Printf("Configuration file already exists at: %s\n", configPath)
				cmd.Println("Skipping generation to avoid overwriting existing configuration.")
				return nil
			}

			if err := config.WriteDefaultConfig(configPath); err != nil {
				return fmt.Errorf("failed to create configuration file: %w", err)
			}

			cmd.Printf("Configuration file created successfully at: %s\n", configPath)
			return nil
		},
	}

	command.Flags().StringVar(&configDir, "config-dir", "",
		"config directory or file path (defaults to OS-specific location)")

	return command
}

func readPassword(prompt string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print(prompt)
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		return string(password), nil
	} else {
		fmt.Fprint(os.Stderr, prompt)
		var password string
		if _, err := fmt.Scanln(&password); err != nil {
			return "", fmt.Errorf("failed to read password from stdin: %w", err)
		}
		return password, nil
	}
}

func RunCreateUserCommand() *cobra.Command {
	var configDir, dataDir, username, password string

	command := &cobra.Command{
		Use:   "create-user",
		Short: "Create the initial user account",
		Long: `Create the initial user account without starting the server.

This command allows you to create the initial user account that is required
for authentication. Only one user account can exist in the system.

If no --config-dir is specified, uses the OS-specific default location:
- Linux/macOS: ~/.config/rui/config.toml
- Windows: %APPDATA%\rui\config.toml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Initialize configuration
			cfg, err := config.New(configDir, buildinfo.Version)
			if err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}

			// Override data directory if provided
			if dataDir != "" {
				cfg.SetDataDir(dataDir)
			}

			db, err := database.New(cfg.GetDatabasePath())
			if err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}
			defer db.Close()

			authService := auth.NewService(db)

			exists, err := authService.IsSetupComplete(context.Background())
			if err != nil {
				return fmt.Errorf("failed to check setup status: %w", err)
			}
			if exists {
				cmd.Println("User account already exists. Only one user account is allowed.")
				return nil
			}

			if username == "" {
				fmt.Print("Enter username: ")
				if _, err := fmt.Scanln(&username); err != nil {
					return fmt.Errorf("failed to read username: %w", err)
				}
			}

			if strings.TrimSpace(username) == "" {
				return fmt.Errorf("username cannot be empty")
			}
			username = strings.TrimSpace(username)

			if password == "" {
				var err error
				password, err = readPassword("Enter password: ")
				if err != nil {
					return err
				}
			}

			if len(password) < 8 {
				return fmt.Errorf("password must be at least 8 characters long")
			}

			user, err := authService.SetupUser(context.Background(), username, password)
			if err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			cmd.Printf("User '%s' created successfully with ID: %d\n", user.Username, user.ID)
			return nil
		},
	}

	command.Flags().StringVar(&configDir, "config-dir", "",
		"config directory or file path (defaults to OS-specific location)")
	command.Flags().StringVar(&dataDir, "data-dir", "",
		"data directory path (defaults to next to config file)")
	command.Flags().StringVar(&username, "username", "",
		"username for the new account")
	command.Flags().StringVar(&password, "password", "",
		"password for the new account (will prompt if not provided)")

	return command
}

func RunChangePasswordCommand() *cobra.Command {
	var configDir, dataDir, username, newPassword string

	command := &cobra.Command{
		Use:   "change-password",
		Short: "Change the password for the existing user",
		Long: `Change the password for the existing user account.

This command allows you to change the password for the existing user account.

If no --config-dir is specified, uses the OS-specific default location:
- Linux/macOS: ~/.config/rui/config.toml
- Windows: %APPDATA%\rui\config.toml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.New(configDir, buildinfo.Version)
			if err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}

			if dataDir != "" {
				cfg.SetDataDir(dataDir)
			}

			dbPath := cfg.GetDatabasePath()
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				return fmt.Errorf("database not found at %s. Create a user first with 'create-user' command", dbPath)
			}

			db, err := database.New(dbPath)
			if err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}
			defer db.Close()

			authService := auth.NewService(db)

			exists, err := authService.IsSetupComplete(context.Background())
			if err != nil {
				return fmt.Errorf("failed to check setup status: %w", err)
			}
			if !exists {
				return fmt.Errorf("no user account found. Create a user first with 'create-user' command")
			}

			if username == "" {
				fmt.Print("Enter username: ")
				if _, err := fmt.Scanln(&username); err != nil {
					return fmt.Errorf("failed to read username: %w", err)
				}
			}

			ctx := context.Background()
			userStore := models.NewUserStore(db)
			user, err := userStore.GetByUsername(ctx, username)
			if err != nil {
				if err == models.ErrUserNotFound {
					return fmt.Errorf("username '%s' not found", username)
				}
				return fmt.Errorf("failed to verify username: %w", err)
			}

			if newPassword == "" {
				var err error
				newPassword, err = readPassword("Enter new password: ")
				if err != nil {
					return err
				}
			}

			if len(newPassword) < 8 {
				return fmt.Errorf("password must be at least 8 characters long")
			}

			hashedPassword, err := auth.HashPassword(newPassword)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}

			userStore = models.NewUserStore(db)
			if err = userStore.UpdatePassword(ctx, user.ID, hashedPassword); err != nil {
				return fmt.Errorf("failed to update password: %w", err)
			}

			cmd.Printf("Password changed successfully for user '%s'\n", user.Username)
			return nil
		},
	}

	command.Flags().StringVar(&configDir, "config-dir", "",
		"config directory or file path (defaults to OS-specific location)")
	command.Flags().StringVar(&dataDir, "data-dir", "",
		"data directory path (defaults to next to config file)")
	command.Flags().StringVar(&username, "username", "",
		"username to verify identity")
	command.Flags().StringVar(&newPassword, "new-password", "",
		"new password (will prompt if not provided)")

	return command
}

func RunUpdateCommand() *cobra.Command {
	var command = &cobra.Command{
		Use:                   "update",
		Short:                 "Self-update is disabled",
		Long:                  `Self-update has been removed from rui. Outbound requests are not made.`,
		DisableFlagsInUseLine: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			updater := update.NewUpdater(update.Config{
				Repository: "autogrr/rui",
				Version:    buildinfo.Version,
			})
			return updater.Run()
		},
	}

	return command
}

type Application struct {
	configDir string
	dataDir   string
	logPath   string
	pprofFlag bool
}

func NewApplication(configDir, dataDir, logPath string, pprofFlag bool) *Application {
	return &Application{
		configDir: configDir,
		dataDir:   dataDir,
		logPath:   logPath,
		pprofFlag: pprofFlag,
	}
}

func (app *Application) runServer() {
	// Initialize configuration
	cfg, err := config.New(app.configDir, buildinfo.Version)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize configuration")
	}

	// Override with CLI flags if provided
	if app.dataDir != "" {
		os.Setenv("RUI__DATA_DIR", app.dataDir)
		cfg.SetDataDir(app.dataDir)
	}
	if app.logPath != "" {
		os.Setenv("RUI__LOG_PATH", app.logPath)
		cfg.Config.LogPath = app.logPath
	}

	if app.pprofFlag {
		cfg.Config.PprofEnabled = true
	}

	cfg.ApplyLogConfig()

	log.Info().Str("version", buildinfo.Version).Msg("Starting rui")

	switch {
	case cfg.Config.IsAuthDisabled():
		if err := cfg.Config.ValidateAuthDisabledConfig(); err != nil {
			log.Fatal().Err(err).Msg("Authentication is disabled but authDisabledAllowedCIDRs is invalid or empty")
		}
		log.Warn().Strs("authDisabledAllowedCIDRs", cfg.Config.AuthDisabledAllowedCIDRs).Msg("Authentication is disabled via RUI__AUTH_DISABLED. Access is restricted to authDisabledAllowedCIDRs. Make sure rui is behind a reverse proxy with its own authentication.")
	case cfg.Config.AuthDisabled != cfg.Config.IAcknowledgeThisIsABadIdea:
		log.Warn().Msg("Only one of RUI__AUTH_DISABLED and RUI__I_ACKNOWLEDGE_THIS_IS_A_BAD_IDEA is set. Authentication remains enabled. Set both to disable authentication.")
	}

	trackerIconService, err := trackericons.NewService(cfg.GetDataDir(), buildinfo.UserAgent)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to prepare tracker icon cache")
	}
	trackericons.SetFetchEnabled(cfg.Config.TrackerIconsFetchEnabled)
	if !cfg.Config.TrackerIconsFetchEnabled {
		log.Info().Msg("Tracker icon remote fetching disabled by configuration")
	}
	// Make tracker icon service globally accessible for background fetching
	trackericons.SetGlobal(trackerIconService)
	cfg.RegisterReloadListener(func(conf *domain.Config) {
		trackericons.SetFetchEnabled(conf.TrackerIconsFetchEnabled)
		log.Debug().Bool("enabled", conf.TrackerIconsFetchEnabled).Msg("Tracker icon fetch setting updated")
	})

	// Initialize database
	db, err := database.New(cfg.GetDatabasePath())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	// Initialize stores
	instanceStore, err := models.NewInstanceStore(db, cfg.GetEncryptionKey())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize instance store")
	}
	instanceReannounceStore := models.NewInstanceReannounceStore(db)
	reannounceSettingsCache := reannounce.NewSettingsCache(instanceReannounceStore)
	if err := reannounceSettingsCache.LoadAll(context.Background()); err != nil {
		log.Warn().Err(err).Msg("Failed to preload reannounce settings cache")
	}

	automationStore := models.NewAutomationStore(db)
	trackerCustomizationStore := models.NewTrackerCustomizationStore(db)
	dashboardSettingsStore := models.NewDashboardSettingsStore(db)
	logExclusionsStore := models.NewLogExclusionsStore(db)

	clientAPIKeyStore := models.NewClientAPIKeyStore(db)
	externalProgramStore := models.NewExternalProgramStore(db)
	arrInstanceStore, err := models.NewArrInstanceStore(db, cfg.GetEncryptionKey())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize ARR instance store")
	}
	arrIDCacheStore := models.NewArrIDCacheStore(db)
	errorStore := models.NewInstanceErrorStore(db)

	// Initialize services
	authService := auth.NewService(db)

	// Initialize qBittorrent client pool
	clientPool, err := qbittorrent.NewClientPool(instanceStore, errorStore)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize client pool")
	}
	defer clientPool.Close()

	// Initialize managers
	syncManager := qbittorrent.NewSyncManager(clientPool, trackerCustomizationStore)

	// Initialize files manager for caching torrent file information
	filesManagerService := filesmanager.NewService(db) // implements qbittorrent.FilesManager
	syncManager.SetFilesManager(filesManagerService)

	// Start background orphan cleanup for torrent files cache
	filesCleanupCtx, filesCleanupCancel := context.WithCancel(context.Background())
	defer filesCleanupCancel()
	filesManagerService.StartOrphanCleanup(filesCleanupCtx,
		&instanceListerAdapter{store: instanceStore},
		&torrentHashAdapter{syncManager: syncManager},
	)

	// Initialize Torznab indexer store
	torznabIndexerStore, err := models.NewTorznabIndexerStore(db, cfg.GetEncryptionKey())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize torznab indexer store")
	}

	// Initialize Torznab torrent cache, search cache and Jackett/Torznab service
	torznabTorrentCache := models.NewTorznabTorrentCacheStore(db)
	torznabSearchCache := models.NewTorznabSearchCacheStore(db)

	// Resolve owner ID for owner-scoped settings (single-user: first user)
	var settingsOwnerID int
	if err := db.QueryRowContext(context.Background(), `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&settingsOwnerID); err != nil {
		log.Warn().Err(err).Msg("No users found; owner-scoped settings will use default TTL")
	}

	cacheTTL := jackett.DefaultSearchCacheTTL
	if settingsOwnerID > 0 {
		if cacheSettings, err := torznabSearchCache.GetSettings(context.Background(), settingsOwnerID); err != nil {
			log.Warn().Err(err).Msg("Using default torznab search cache TTL (failed to load settings)")
		} else if cacheSettings != nil && cacheSettings.TTLMinutes > 0 {
			cacheTTL = time.Duration(cacheSettings.TTLMinutes) * time.Minute
			if cacheTTL < jackett.MinSearchCacheTTL {
				cacheTTL = jackett.MinSearchCacheTTL
			}

			if rebased, err := torznabSearchCache.RebaseTTL(context.Background(), int(cacheTTL/time.Minute)); err != nil {
				log.Warn().Err(err).Msg("Failed to rebase torznab search cache TTL to persisted settings")
			} else if rebased > 0 {
				log.Info().
					Int64("updatedRows", rebased).
					Float64("ttlHours", cacheTTL.Hours()).
					Msg("Rebased torznab search cache entries to persisted TTL")
			}
		}
	}
	jackettService := jackett.NewService(
		torznabIndexerStore,
		jackett.WithTorrentCache(torznabTorrentCache),
		jackett.WithOwnerID(settingsOwnerID),
		jackett.WithSearchCache(torznabSearchCache, jackett.SearchCacheConfig{
			TTL: cacheTTL,
		}),
		jackett.WithSearchHistory(0),   // Use default capacity (500 entries)
		jackett.WithIndexerOutcomes(0), // Use default capacity (1000 entries)
	)
	log.Info().Msg("Torznab/Jackett service initialized")

	// Initialize ARR service for Sonarr/Radarr ID lookup
	arrService := arr.NewService(arrInstanceStore, arrIDCacheStore)
	log.Info().Msg("ARR service initialized")

	// Initialize library service (native rls+expr title matching)
	libraryTitleStore := models.NewLibraryTitleStore(db)
	libraryRuleStore := models.NewLibraryRuleStore(db)
	libraryService := library.New(libraryTitleStore, libraryRuleStore, arrService)
	log.Info().Msg("Library service initialized")

	// Initialize intake pipeline service
	intakePipelineStore := models.NewIntakePipelineStore(db)
	intakeEventStore := models.NewIntakeEventStore(db)
	intakeService := intake.New(intakePipelineStore, intakeEventStore, libraryService, arrService)
	log.Info().Msg("Intake pipeline service initialized")

	// Initialize automation activity store and external programs service
	automationActivityStore := models.NewAutomationActivityStore(db)
	externalProgramService := externalprograms.NewService(externalProgramStore, automationActivityStore, cfg.Config)
	notificationTargetStore := models.NewNotificationTargetStore(db)
	notificationService := notifications.NewService(notificationTargetStore, instanceStore, log.Logger.With().Str("module", "notifications").Logger())
	notificationCtx, notificationCancel := context.WithCancel(context.Background())
	defer notificationCancel()
	if notificationService != nil {
		notificationService.Start(notificationCtx)
	}

	// Initialize cross-seed automation store and service
	crossSeedStore, err := models.NewCrossSeedStore(db, cfg.GetEncryptionKey())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize cross-seed store")
	}
	instanceCrossSeedCompletionStore := models.NewInstanceCrossSeedCompletionStore(db)
	crossSeedBlocklistStore := models.NewCrossSeedBlocklistStore(db)
	crossSeedService := crossseed.NewService(
		instanceStore,
		syncManager,
		filesManagerService,
		crossSeedStore,
		crossSeedBlocklistStore,
		jackettService,
		arrService,
		externalProgramStore,
		externalProgramService,
		instanceCrossSeedCompletionStore,
		trackerCustomizationStore,
		notificationService,
		cfg.Config.CrossSeedRecoverErroredTorrents,
	)
	reannounceService := reannounce.NewService(reannounce.DefaultConfig(), instanceStore, instanceReannounceStore, reannounceSettingsCache, clientPool, syncManager)
	automationService := automations.NewService(automations.DefaultConfig(), instanceStore, automationStore, automationActivityStore, trackerCustomizationStore, syncManager, notificationService, externalProgramService)

	orphanScanStore := models.NewOrphanScanStore(db)
	orphanScanService := orphanscan.NewService(orphanscan.DefaultConfig(), instanceStore, orphanScanStore, syncManager, notificationService)

	dirScanStore := models.NewDirScanStore(db)
	dirScanService := dirscan.NewService(dirscan.DefaultConfig(), dirScanStore, crossSeedStore, instanceStore, syncManager, jackettService, arrService, trackerCustomizationStore, notificationService)

	syncManager.SetTorrentCompletionHandler(func(ctx context.Context, instanceID int, torrent qbt.Torrent) {
		crossSeedService.HandleTorrentCompletion(ctx, instanceID, torrent)
		notificationService.Notify(ctx, buildTorrentCompletedEvent(syncManager, instanceID, torrent))
	})

	syncManager.SetTorrentAddedHandler(func(ctx context.Context, instanceID int, torrent qbt.Torrent) {
		notifyTorrentAddedWithDelay(ctx, syncManager, notificationService, instanceID, torrent)
	})

	automationCtx, automationCancel := context.WithCancel(context.Background())
	defer func() {
		automationCancel()
		crossSeedService.StopAutomation()
	}()

	reannounceCtx, reannounceCancel := context.WithCancel(context.Background())
	defer reannounceCancel()
	reannounceService.Start(reannounceCtx)

	automationsCtx, automationsCancel := context.WithCancel(context.Background())
	defer automationsCancel()
	automationService.Start(automationsCtx)

	orphanScanCtx, orphanScanCancel := context.WithCancel(context.Background())
	defer orphanScanCancel()
	orphanScanService.Start(orphanScanCtx)

	dirScanCtx, dirScanCancel := context.WithCancel(context.Background())
	defer dirScanCancel()
	if err := dirScanService.Start(dirScanCtx); err != nil {
		log.Error().Err(err).Msg("failed to start dirscan service")
	}

	backupStore := models.NewBackupStore(db)
	backupService := backups.NewService(backupStore, syncManager, jackettService, backups.Config{DataDir: cfg.GetDataDir()}, notificationService)
	backupService.Start(context.Background())
	defer backupService.Stop()

	updateService := update.NewService(log.Logger, cfg.Config.CheckForUpdates, buildinfo.Version, buildinfo.UserAgent)
	cfg.RegisterReloadListener(func(conf *domain.Config) {
		updateService.SetEnabled(conf.CheckForUpdates)
	})
	updateCtx, cancelUpdate := context.WithCancel(context.Background())
	defer cancelUpdate()
	updateService.Start(updateCtx)

	// Initialize client connections for all active instances on startup
	go func() {
		listCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		instances, err := instanceStore.List(listCtx)
		cancel()

		if err != nil {
			log.Error().Err(err).Msg("Failed to get instances for startup connection")
			return
		}

		// Connect to instances in parallel with separate timeouts
		for _, instance := range instances {
			if !instance.IsActive {
				log.Debug().
					Int("instanceID", instance.ID).
					Str("instanceName", instance.Name).
					Msg("Skipping startup connection for disabled instance")
				continue
			}

			go func(instanceID int) {
				// Use separate context for each connection attempt with longer timeout
				connCtx, connCancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer connCancel()

				// Trigger connection by trying to get client
				// This will populate the pool for GetClientOffline calls
				_, err := clientPool.GetClient(connCtx, instanceID)
				if err != nil {
					log.Debug().Err(err).Int("instanceID", instanceID).Msg("Failed to connect to instance on startup")
				} else {
					log.Debug().Int("instanceID", instanceID).Msg("Successfully connected to instance on startup")
				}
			}(instance.ID)
		}
	}()

	// Initialize session manager
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.New(db)
	sessionManager.Lifetime = 24 * time.Hour * 30 // 30 days
	sessionManager.Cookie.Name = auth.SessionName
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Secure = false // Will be set to true when HTTPS is detected
	sessionManager.Cookie.Persist = false

	// Start server in goroutine
	httpServer := api.NewServer(&api.Dependencies{
		Config:                           cfg,
		Version:                          buildinfo.Version,
		AuthService:                      authService,
		SessionManager:                   sessionManager,
		InstanceStore:                    instanceStore,
		InstanceReannounce:               instanceReannounceStore,
		ReannounceCache:                  reannounceSettingsCache,
		ReannounceService:                reannounceService,
		ClientAPIKeyStore:                clientAPIKeyStore,
		ExternalProgramStore:             externalProgramStore,
		ExternalProgramService:           externalProgramService,
		ClientPool:                       clientPool,
		SyncManager:                      syncManager,
		UpdateService:                    updateService,
		TrackerIconService:               trackerIconService,
		BackupService:                    backupService,
		FilesManager:                     filesManagerService,
		CrossSeedService:                 crossSeedService,
		JackettService:                   jackettService,
		TorznabIndexerStore:              torznabIndexerStore,
		AutomationStore:                  automationStore,
		AutomationActivityStore:          automationActivityStore,
		AutomationService:                automationService,
		TrackerCustomizationStore:        trackerCustomizationStore,
		DashboardSettingsStore:           dashboardSettingsStore,
		LogExclusionsStore:               logExclusionsStore,
		NotificationTargetStore:          notificationTargetStore,
		NotificationService:              notificationService,
		InstanceCrossSeedCompletionStore: instanceCrossSeedCompletionStore,
		OrphanScanStore:                  orphanScanStore,
		OrphanScanService:                orphanScanService,
		DirScanService:                   dirScanService,
		ArrInstanceStore:                 arrInstanceStore,
		ArrService:                       arrService,
		LibraryTitleStore:                libraryTitleStore,
		LibraryRuleStore:                 libraryRuleStore,
		LibraryService:                   libraryService,
		IntakePipelineStore:              intakePipelineStore,
		IntakeEventStore:                 intakeEventStore,
		IntakeService:                    intakeService,
	})

	// Reconcile any cross-seed runs left in 'running' status from a previous crash/restart.
	// Use a short timeout so a locked DB can't hang startup.
	reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reconcileCancel()
	crossSeedService.ReconcileInterruptedRuns(reconcileCtx)

	errorChannel := make(chan error)
	serverReady := make(chan struct{}, 1)
	go func() {
		if err := httpServer.ListenAndServeReady(serverReady); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorChannel <- err
		}
	}()

	select {
	case <-serverReady:
		crossSeedService.StartAutomation(automationCtx)
	case err := <-errorChannel:
		log.Fatal().Err(err).Msg("failed to start HTTP server")
	}

	if cfg.Config.MetricsEnabled {
		metricsManager := metrics.NewMetricsManager(syncManager, clientPool, trackerCustomizationStore)

		// Start metrics server on separate port
		go func() {
			metricsServer := metrics.NewMetricsServer(
				metricsManager,
				cfg.Config.MetricsHost,
				cfg.Config.MetricsPort,
				cfg.Config.MetricsBasicAuthUsers,
			)

			errorChannel <- metricsServer.ListenAndServe()
		}()
	}

	// Start profiling server if enabled
	if cfg.Config.PprofEnabled {
		go func() {
			log.Info().Msg("Starting pprof server on :6060")
			log.Info().Msg("Access profiling at: http://localhost:6060/debug/pprof/")
			if err := http.ListenAndServe(":6060", nil); err != nil {
				log.Error().Err(err).Msg("Profiling server failed")
			}
		}()
	}

	// Wait for interrupt signal to gracefully shutdown the server
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info().Msgf("got signal %v, shutting down server", sig.String())
	case err := <-errorChannel:
		log.Error().Err(err).Msg("got unexpected error from server")
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		//log.Fatal().Err(err).Msg("Server forced to shutdown")
		log.Error().Err(err).Msg("got error during graceful http shutdown")

		os.Exit(1)
	}

	//if err := srv.Shutdown(context.Background()); err != nil {
	//	log.Error().Err(err).Msg("got error during graceful http shutdown")
	//
	//	os.Exit(1)
	//}

	os.Exit(0)

	//// Wait for interrupt signal to gracefully shutdown the server
	//quit := make(chan os.Signal, 1)
	//signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	//<-quit
	//log.Info().Msg("Shutting down server...")
	//
	//// Graceful shutdown with timeout
	//ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//defer cancel()
	//
	//if err := httpServer.Close(ctx); err != nil {
	//	log.Fatal().Err(err).Msg("Server forced to shutdown")
	//}
	//
	//log.Info().Msg("Server stopped")
}

// instanceListerAdapter implements filesmanager.InstanceLister
type instanceListerAdapter struct {
	store *models.InstanceStore
}

func (a *instanceListerAdapter) ListInstanceIDs(ctx context.Context) ([]int, error) {
	instances, err := a.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	// Only return active instances - disabled ones can't be queried for current hashes
	ids := make([]int, 0, len(instances))
	for _, inst := range instances {
		if inst.IsActive {
			ids = append(ids, inst.ID)
		}
	}
	return ids, nil
}

// torrentHashAdapter implements filesmanager.TorrentHashProvider
type torrentHashAdapter struct {
	syncManager *qbittorrent.SyncManager
}

func (a *torrentHashAdapter) GetAllTorrentHashes(ctx context.Context, instanceID int) ([]string, error) {
	torrents, err := a.syncManager.GetAllTorrents(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get torrents: %w", err)
	}
	hashes := make([]string, len(torrents))
	for i := range torrents {
		hashes[i] = qbt.Deref(torrents[i].Hash)
	}
	return hashes, nil
}
