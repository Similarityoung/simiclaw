package gateway

import (
	"context"
	"errors"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/messages"
	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

type Options struct {
	ConfigPath string
	Workspace  string
	Listen     string
}

func NewCommand() *cobra.Command {
	opts := Options{}
	cmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"gateway"},
		Short:   messages.Command.GatewayShort,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(opts)
		},
	}
	cmd.Flags().StringVar(&opts.ConfigPath, "config", "", messages.Flag.ConfigJSON)
	cmd.Flags().StringVar(&opts.Workspace, "workspace", "", messages.Flag.WorkspaceOverride)
	cmd.Flags().StringVar(&opts.Listen, "listen", "", messages.Flag.ListenOverride)
	return cmd
}

func run(opts Options) error {
	// 先加载 .env（可选），让环境变量覆盖优先于 JSON 配置
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}

	if opts.Workspace != "" {
		cfg.Workspace = opts.Workspace
	}
	if opts.Listen != "" {
		cfg.ListenAddr = opts.Listen
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}
	if err := common.SetupLogger(cfg.LogLevel); err != nil {
		return err
	}
	logger := logging.L("cmd")

	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("simiclaw serving", logging.String("addr", cfg.ListenAddr), logging.String("workspace", cfg.Workspace))
	err = app.RunHTTPServer(ctx)
	if err != nil && (errors.Is(err, context.Canceled) || err.Error() == "http: Server closed") {
		logger.Info("simiclaw stopped")
		return nil
	}
	if err != nil {
		if !errors.Is(err, bootstrap.ErrStartup) {
			logger.Error("simiclaw serve failed", logging.Error(err))
		}
		return err
	}
	logger.Info("simiclaw stopped")
	return err
}
