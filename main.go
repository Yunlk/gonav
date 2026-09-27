// Command gonav 启动内部网址导航服务。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gonav/internal/catalog"
	"gonav/internal/gatewayclient"
	"gonav/internal/grpcsvc"
	"gonav/internal/stats"
	"gonav/internal/web"
)

// version 可在构建时通过 -ldflags "-X main.version=..." 注入。
var version = "0.1.0"

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP 监听地址，例如 :8080 或 0.0.0.0:8123")
		grpcAddr  = flag.String("grpc-addr", ":9190", "gRPC 监听地址（供主网关调用，明文 h2c）")
		grpcOff   = flag.Bool("grpc-disable", false, "关闭 gRPC 服务")
		configOut = flag.String("config", "config/links.json", "导航链接配置(JSON)路径")
		statsOut  = flag.String("stats", "data/stats.json", "访问统计落盘路径")
		debug     = flag.Bool("debug", false, "开发模式：模板/静态资源每次请求重新读取，禁用缓存")
		webRoot   = flag.String("web-root", "internal/web", "开发模式下的模板与资源根目录")
		watch     = flag.Duration("watch", 3*time.Second, "配置热更新检查间隔，0 表示关闭")
		gwAddr    = flag.String("gateway-addr", "", "主网关地址，用于反向回调（留空即关闭）")
		gwOn      = flag.Bool("gateway-callback", false, "开启对主网关的反向回调（契约未定，目前是骨架）")
		scriptOut = flag.String("userscripts", "data/userscripts.json", "极客模式沙箱用户脚本落盘路径，留空关闭")
	)
	flag.Parse()

	startedAt := time.Now()

	gateway := gatewayclient.New(gatewayclient.Options{Addr: *gwAddr, Enabled: *gwOn})

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	store, err := catalog.NewStore(*configOut)
	if err != nil {
		slog.Error("加载链接配置失败", "config", *configOut, "err", err)
		os.Exit(1)
	}

	st, err := stats.Open(*statsOut)
	if err != nil {
		// 统计不可用不应阻塞导航页启动。
		slog.Warn("访问统计初始化失败，本次运行不记录点击", "stats", *statsOut, "err", err)
		st = nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *watch > 0 {
		go store.Watch(ctx, *watch, nil)
	}
	if st != nil {
		go st.Run(ctx, 30*time.Second)
	}

	srv, err := web.New(web.Options{
		Store:      store,
		Stats:      st,
		Debug:      *debug,
		DevRoot:    *webRoot,
		Version:    version,
		Gateway:    gateway,
		ScriptPath: *scriptOut,
	})
	if err != nil {
		slog.Error("初始化 HTTP 服务失败", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	// gRPC 与 HTTP 同进程、不同端口：共用同一份 catalog/stats，不存在两套数据源。
	if !*grpcOff {
		g := grpcsvc.New(grpcsvc.Options{
			Store:     store,
			Stats:     st,
			Version:   version,
			HTTPAddr:  *addr,
			GRPCAddr:  *grpcAddr,
			StartedAt: startedAt,
			Gateway:   gateway,
		})
		go func() {
			if err := g.Serve(ctx, *grpcAddr); err != nil {
				slog.Error("gRPC 服务异常退出", "addr", *grpcAddr, "err", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	absConfig, _ := filepath.Abs(*configOut)
	slog.Info("gonav 已启动",
		"addr", *addr,
		"grpc", map[bool]string{true: "disabled", false: *grpcAddr}[*grpcOff],
		"config", absConfig,
		"categories", store.Current().CategoryCount(),
		"links", store.Current().LinkCount(),
		"debug", *debug,
		"version", version,
		"gatewayCallback", gateway.Describe(),
	)

	select {
	case err := <-errCh:
		slog.Error("HTTP 服务异常退出", "err", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("收到退出信号，正在优雅关闭…")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("优雅关闭超时", "err", err)
	}
	if st != nil {
		if err := st.Flush(); err != nil {
			slog.Warn("统计落盘失败", "err", err)
		}
	}
	slog.Info("gonav 已停止")
}
