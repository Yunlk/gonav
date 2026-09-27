// Package gatewayclient 是 navkit 反向回调主网关的客户端骨架。
//
// 目前**默认关闭**：主网关的服务名、鉴权方式与 proto 还没定，所以这里只定义
// 事件模型与调用点，接上真实契约时只需要替换 Enabled 分支里的实现，
// 调用方（main.go / grpcsvc）不用改。
package gatewayclient

import (
	"context"
	"log/slog"
	"time"
)

// EventKind 是回调事件种类。
type EventKind string

// 支持的事件。
const (
	// EventCatalogChanged 在配置被替换（网关下发或热更新）后发出。
	EventCatalogChanged EventKind = "catalog.changed"
	// EventClickRecorded 在入口被点击后发出（按批发送，避免打爆网关）。
	EventClickRecorded EventKind = "click.recorded"
)

// Event 是回调载荷。
type Event struct {
	Kind        EventKind `json:"kind"`
	Revision    uint64    `json:"revision,omitempty"`
	ContentHash string    `json:"contentHash,omitempty"`
	Source      string    `json:"source,omitempty"`
	LinkID      string    `json:"linkId,omitempty"`
	Hits        int       `json:"hits,omitempty"`
	OccurredAt  time.Time `json:"occurredAt"`
}

// Options 配置回调客户端。
type Options struct {
	// Addr 是主网关的 gRPC 地址；为空或 Enabled=false 时客户端退化为 no-op。
	Addr    string
	Token   string
	Enabled bool
	Timeout time.Duration
}

// Client 发送事件。
type Client interface {
	Emit(ctx context.Context, event Event) error
	// Describe 用于 /healthz 与启动日志展示当前回调状态。
	Describe() string
}

// New 构造客户端。契约未定时返回 no-op 实现，调用方无需分支判断。
func New(opts Options) Client {
	if !opts.Enabled || opts.Addr == "" {
		return noopClient{}
	}
	return &grpcClient{opts: opts}
}

type noopClient struct{}

func (noopClient) Emit(context.Context, Event) error { return nil }
func (noopClient) Describe() string                  { return "disabled" }

type grpcClient struct {
	opts Options
}

// Emit 目前只做记录。
//
// TODO(gateway): 主网关给出 proto 后，在这里建立/复用 grpc.ClientConn，
// 把事件映射成对应的 RPC（含 Token metadata 与超时），并按 Event.Kind 分流。
func (c *grpcClient) Emit(ctx context.Context, event Event) error {
	timeout := c.opts.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	_, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	slog.Debug("gateway callback (骨架，未真正发送)",
		"addr", c.opts.Addr,
		"kind", event.Kind,
		"revision", event.Revision,
		"linkId", event.LinkID,
	)
	return nil
}

func (c *grpcClient) Describe() string { return "enabled → " + c.opts.Addr }
