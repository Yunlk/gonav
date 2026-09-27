package grpcsvc

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// TODO(auth): 目前内网免鉴权。网关改走 token / mTLS 时，只需在这里加一个
// 拦截器并塞进链上，业务方法签名不用动。
func logUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	code := status.Code(err)

	attrs := []any{
		"method", info.FullMethod,
		"code", code.String(),
		"dur", time.Since(start).Round(time.Millisecond).String(),
	}
	if addr := peerAddr(ctx); addr != "" {
		attrs = append(attrs, "peer", addr)
	}
	if code == codes.OK {
		slog.Debug("grpc", attrs...)
	} else {
		attrs = append(attrs, "err", err.Error())
		slog.Warn("grpc", attrs...)
	}
	return resp, err
}

func recoverUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("gRPC 处理 panic", "method", info.FullMethod, "panic", rec, "stack", string(debug.Stack()))
			err = status.Error(codes.Internal, "服务内部错误")
		}
	}()
	return handler(ctx, req)
}

func logStream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	slog.Debug("grpc-stream",
		"method", info.FullMethod,
		"code", status.Code(err).String(),
		"dur", time.Since(start).Round(time.Millisecond).String(),
	)
	return err
}

func recoverStream(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("gRPC 流处理 panic", "method", info.FullMethod, "panic", rec, "stack", string(debug.Stack()))
			err = status.Error(codes.Internal, "服务内部错误")
		}
	}()
	return handler(srv, ss)
}

func peerAddr(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return p.Addr.String()
	}
	return ""
}
