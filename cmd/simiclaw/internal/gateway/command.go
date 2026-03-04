package gateway

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/config"
)

// Run 解析启动参数并运行网关 HTTP 服务，处理进程退出信号。
func Run(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "config json file")
	workspaceOverride := fs.String("workspace", "", "workspace override")
	listenOverride := fs.String("listen", "", "listen address override")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if *configPath != "" {
			return err
		}
		cfg = config.Default()
	}

	// 命令行参数覆盖配置文件中的对应项，优先级最高。
	if *workspaceOverride != "" {
		cfg.Workspace = *workspaceOverride
	}
	if *listenOverride != "" {
		cfg.ListenAddr = *listenOverride
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}

	app, err := api.NewApp(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("simiclaw serving", "addr", cfg.ListenAddr, "workspace", cfg.Workspace)
	err = app.RunHTTPServer(ctx)
	if err != nil && (errors.Is(err, context.Canceled) || err.Error() == "http: Server closed") {
		return nil
	}
	return err
}
