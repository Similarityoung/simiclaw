package gateway

import (
	"context"
	"errors"
	"flag"
	"os/signal"
	"syscall"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/bootstrap"
	"github.com/similarityyoung/simiclaw/pkg/config"
	"github.com/similarityyoung/simiclaw/pkg/logging"
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

	// 先加载 .env（可选），让环境变量覆盖优先于 JSON 配置
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
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
	if err := common.SetupLogger(cfg.LogLevel); err != nil {
		return err
	}

	app, err := bootstrap.NewApp(cfg)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logging.L("cmd").Info("simiclaw serving", logging.String("addr", cfg.ListenAddr), logging.String("workspace", cfg.Workspace))
	err = app.RunHTTPServer(ctx)
	if err != nil && (errors.Is(err, context.Canceled) || err.Error() == "http: Server closed") {
		return nil
	}
	return err
}
