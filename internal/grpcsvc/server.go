// Package grpcsvc 实现供主网关调用的 navkit gRPC 服务。
//
// 设计要点：
//   - 与 HTTP REST 共用同一份 catalog/stats，不存在两套数据源；
//   - 每条响应都带 revision 与 content_hash，网关据此判断过期与断点续推；
//   - M2 的方法已声明但返回 Unimplemented，能力清单通过 GetServerInfo 暴露，
//     网关不必靠试错判断哪些可用。
package grpcsvc

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"net"
	"time"

	navkitv1 "gonav/gen/navkit/v1"
	"gonav/internal/catalog"
	"gonav/internal/gatewayclient"
	"gonav/internal/stats"
)

// capabilities 列出当前真正可用的方法，避免网关踩到 Unimplemented。
var capabilities = []string{
	"navkit.v1.NavkitService/Health",
	"navkit.v1.NavkitService/GetServerInfo",
	"navkit.v1.NavkitService/GetCatalog",
	"navkit.v1.NavkitService/ListCategories",
	"navkit.v1.NavkitService/Search",
	"navkit.v1.NavkitService/GetStats",
	"navkit.v1.NavkitService/PutCatalog",
	"navkit.v1.NavkitService/PatchLink",
	"navkit.v1.NavkitService/WatchCatalog",
}

// Options 是构造 gRPC 服务所需的依赖。
type Options struct {
	Store     *catalog.Store
	Stats     *stats.Store
	Version   string
	HTTPAddr  string
	GRPCAddr  string
	StartedAt time.Time
	// Gateway 是反向回调客户端；为空表示不回调。
	Gateway gatewayclient.Client
}

// Server 实现 navkitv1.NavkitServiceServer。
type Server struct {
	navkitv1.UnimplementedNavkitServiceServer

	opts Options
}

// New 构造服务。
func New(opts Options) *Server {
	if opts.StartedAt.IsZero() {
		opts.StartedAt = time.Now()
	}
	return &Server{opts: opts}
}

// Serve 在 addr 上以明文 h2c 提供服务，ctx 结束时优雅停机。
func (s *Server) Serve(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(recoverUnary, logUnary),
		grpc.ChainStreamInterceptor(recoverStream, logStream),
	)
	navkitv1.RegisterNavkitServiceServer(srv, s)
	healthpb.RegisterHealthServer(srv, health.NewServer())
	// 注册反射，网关侧可以直接 grpcurl list / describe
	reflection.Register(srv)

	go func() {
		<-ctx.Done()
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			srv.Stop()
		}
	}()

	slog.Info("navkit gRPC 已启动", "addr", addr, "services", len(capabilities))
	err = srv.Serve(lis)
	if errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------- M1 方法

// Health 返回存活与降级状态。配置热更新失败时为 degraded，但仍在服务上一版可用配置。
func (s *Server) Health(ctx context.Context, _ *navkitv1.HealthRequest) (*navkitv1.HealthResponse, error) {
	c := s.opts.Store.Current()
	resp := &navkitv1.HealthResponse{
		Status:        "ok",
		Revision:      c.Revision(),
		LinkCount:     int32(c.LinkCount()),
		CategoryCount: int32(c.CategoryCount()),
		UpdatedAt:     timestamppb.New(c.UpdatedAt()),
	}
	if err := s.opts.Store.LastError(); err != nil {
		resp.Status = "degraded"
		resp.LastError = err.Error()
	}
	return resp, nil
}

// GetServerInfo 返回版本、地址与能力清单。
func (s *Server) GetServerInfo(ctx context.Context, _ *navkitv1.GetServerInfoRequest) (*navkitv1.GetServerInfoResponse, error) {
	c := s.opts.Store.Current()
	return &navkitv1.GetServerInfoResponse{
		Version:      s.opts.Version,
		HttpAddr:     s.opts.HTTPAddr,
		GrpcAddr:     s.opts.GRPCAddr,
		ConfigPath:   c.SourcePath(),
		Revision:     c.Revision(),
		StartedAt:    timestamppb.New(s.opts.StartedAt),
		Capabilities: append([]string(nil), capabilities...),
	}, nil
}

// GetCatalog 返回全量配置；known_revision 与当前一致时只回 not_modified。
func (s *Server) GetCatalog(ctx context.Context, req *navkitv1.GetCatalogRequest) (*navkitv1.GetCatalogResponse, error) {
	c := s.opts.Store.Current()

	if req.GetKnownRevision() != 0 && req.GetKnownRevision() == c.Revision() {
		return &navkitv1.GetCatalogResponse{NotModified: true}, nil
	}

	if id := req.GetCategoryId(); id != "" {
		cat, ok := c.Category(id)
		if !ok {
			return nil, status.Errorf(codes.NotFound, "分类不存在: %s", id)
		}
		return &navkitv1.GetCatalogResponse{
			Catalog: &navkitv1.Catalog{
				Site:       siteToProto(c),
				Categories: []*navkitv1.Category{categoryToProto(cat, req.GetIncludeLinks())},
				Meta:       metaToProto(c),
			},
		}, nil
	}

	cats := make([]*navkitv1.Category, 0, len(c.Categories))
	for _, cat := range c.Categories {
		cats = append(cats, categoryToProto(cat, req.GetIncludeLinks()))
	}
	return &navkitv1.GetCatalogResponse{
		Catalog: &navkitv1.Catalog{
			Site:       siteToProto(c),
			Categories: cats,
			Meta:       metaToProto(c),
		},
	}, nil
}

// ListCategories 只返回分类骨架（不含入口），用于网关构建分类切换器。
func (s *Server) ListCategories(ctx context.Context, _ *navkitv1.ListCategoriesRequest) (*navkitv1.ListCategoriesResponse, error) {
	c := s.opts.Store.Current()
	cats := make([]*navkitv1.Category, 0, len(c.Categories))
	for _, cat := range c.Categories {
		cats = append(cats, categoryToProto(cat, false))
	}
	return &navkitv1.ListCategoriesResponse{Categories: cats, Meta: metaToProto(c)}, nil
}

// Search 复用服务端加权检索（含拼音与首字母）。
func (s *Server) Search(ctx context.Context, req *navkitv1.SearchRequest) (*navkitv1.SearchResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		return nil, status.Errorf(codes.InvalidArgument, "limit 上限 200，收到 %d", limit)
	}

	c := s.opts.Store.Current()
	matches := c.Search(req.GetQuery(), 0)

	out := make([]*navkitv1.SearchMatch, 0, min(limit, len(matches)))
	for _, m := range matches {
		if id := req.GetCategoryId(); id != "" && m.Link.CategoryID != id {
			continue
		}
		out = append(out, &navkitv1.SearchMatch{Link: linkToProto(m.Link), Score: int32(m.Score)})
		if len(out) == limit {
			break
		}
	}

	return &navkitv1.SearchResponse{Query: req.GetQuery(), Matches: out, Revision: c.Revision()}, nil
}

// ---------------------------------------------------------------- M2 方法

// PutCatalog 由网关下发全量配置：同一套校验 + 原子落盘 + 热更新 + 版本递增。
func (s *Server) PutCatalog(ctx context.Context, req *navkitv1.PutCatalogRequest) (*navkitv1.PutCatalogResponse, error) {
	if len(req.GetCategories()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "categories 不能为空")
	}

	c := catalogFromProto(req.GetSite(), req.GetCategories())
	raw, err := catalog.Marshal(c)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "序列化配置失败: %v", err)
	}

	updated, err := s.opts.Store.ReplaceRaw(raw, catalog.SourceGateway, req.GetExpectedRevision(), req.GetDryRun())
	if err != nil {
		return nil, toStatus(err)
	}
	if !req.GetDryRun() {
		s.emitCatalogChanged(updated)
	}

	return &navkitv1.PutCatalogResponse{
		Revision:      updated.Revision(),
		ContentHash:   updated.ContentHash(),
		LinkCount:     int32(updated.LinkCount()),
		CategoryCount: int32(updated.CategoryCount()),
	}, nil
}

// PatchLink 增删改单条入口。
func (s *Server) PatchLink(ctx context.Context, req *navkitv1.PatchLinkRequest) (*navkitv1.PatchLinkResponse, error) {
	var action catalog.PutAction
	switch req.GetAction() {
	case navkitv1.PatchLinkRequest_ACTION_UPSERT:
		action = catalog.ActionUpsert
	case navkitv1.PatchLinkRequest_ACTION_DELETE:
		action = catalog.ActionDelete
	default:
		return nil, status.Error(codes.InvalidArgument, "action 必须是 UPSERT 或 DELETE")
	}

	var link catalog.Link
	if proto := req.GetLink(); proto != nil {
		link = catalog.Link{
			ID:     proto.GetId(),
			Name:   proto.GetName(),
			URL:    proto.GetUrl(),
			Icon:   proto.GetIcon(),
			Desc:   proto.GetDesc(),
			Tags:   proto.GetTags(),
			Pinned: proto.GetPinned(),
		}
	}

	updated, err := s.opts.Store.PatchLink(action, req.GetCategoryId(), link)
	if err != nil {
		return nil, toStatus(err)
	}
	s.emitCatalogChanged(updated)
	return &navkitv1.PatchLinkResponse{Revision: updated.Revision()}, nil
}

// emitCatalogChanged 反向通知主网关「配置变了」。
// 契约未定时客户端是 no-op，所以这里可以无条件调用。
func (s *Server) emitCatalogChanged(c *catalog.Catalog) {
	if s.opts.Gateway == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.opts.Gateway.Emit(ctx, gatewayclient.Event{
			Kind:        gatewayclient.EventCatalogChanged,
			Revision:    c.Revision(),
			ContentHash: c.ContentHash(),
			Source:      c.Source(),
			OccurredAt:  time.Now(),
		})
		if err != nil {
			slog.Warn("回调主网关失败", "err", err)
		}
	}()
}

// GetStats 返回累计点击与 Top N（M2 实现）。
func (s *Server) GetStats(ctx context.Context, req *navkitv1.GetStatsRequest) (*navkitv1.GetStatsResponse, error) {
	if s.opts.Stats == nil {
		return &navkitv1.GetStatsResponse{}, nil
	}
	top := int(req.GetTop())
	if top <= 0 {
		top = 20
	}
	c := s.opts.Store.Current()
	entries := s.opts.Stats.Top(top)
	out := make([]*navkitv1.StatsEntry, 0, len(entries))
	for _, e := range entries {
		item := &navkitv1.StatsEntry{Id: e.ID, Hits: e.Hits}
		if link, ok := c.Link(e.ID); ok {
			item.Name = link.Name
			item.Url = link.URL
		}
		out = append(out, item)
	}
	return &navkitv1.GetStatsResponse{Total: s.opts.Stats.Total(), Top: out}, nil
}

// WatchCatalog 订阅配置变更：落后时立刻补推当前快照，之后每次变更推送新快照，
// 中间每 30 秒发一次心跳保活。
func (s *Server) WatchCatalog(req *navkitv1.WatchCatalogRequest, stream grpc.ServerStreamingServer[navkitv1.WatchCatalogResponse]) error {
	ctx := stream.Context()

	current := s.opts.Store.Current()
	if from := req.GetFromRevision(); from != 0 && from < current.Revision() {
		if err := stream.Send(watchEvent(current, false)); err != nil {
			return err
		}
	}

	// 缓冲 1：订阅者来不及消费时只保留最新一份快照，避免堆积过期中间态
	events := make(chan *catalog.Catalog, 1)
	cancel := s.opts.Store.Subscribe(func(c *catalog.Catalog) {
		select {
		case events <- c:
		default:
			select {
			case <-events:
			default:
			}
			select {
			case events <- c:
			default:
			}
		}
	})
	defer cancel()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case updated := <-events:
			if err := stream.Send(watchEvent(updated, false)); err != nil {
				return err
			}
		case <-heartbeat.C:
			if err := stream.Send(&navkitv1.WatchCatalogResponse{
				Revision:  s.opts.Store.Revision(),
				Heartbeat: true,
			}); err != nil {
				return err
			}
		}
	}
}

// ---------------------------------------------------------------- 辅助

func watchEvent(c *catalog.Catalog, heartbeat bool) *navkitv1.WatchCatalogResponse {
	event := &navkitv1.WatchCatalogResponse{
		Revision:    c.Revision(),
		ContentHash: c.ContentHash(),
		UpdatedAt:   timestamppb.New(c.UpdatedAt()),
		Heartbeat:   heartbeat,
	}
	if !heartbeat {
		cats := make([]*navkitv1.Category, 0, len(c.Categories))
		for _, cat := range c.Categories {
			cats = append(cats, categoryToProto(cat, true))
		}
		event.Catalog = &navkitv1.Catalog{
			Site:       siteToProto(c),
			Categories: cats,
			Meta:       metaToProto(c),
		}
	}
	return event
}

// toStatus 把 catalog 层的错误映射成 gRPC 状态码，让网关能据此分支处理。
func toStatus(err error) error {
	switch {
	case errors.Is(err, catalog.ErrRevisionConflict):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, catalog.ErrCategoryNotFound), errors.Is(err, catalog.ErrLinkNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		// 校验失败一律算参数问题，消息里带具体位置，网关可直接转给运维
		return status.Error(codes.InvalidArgument, err.Error())
	}
}

// catalogFromProto 把网关下发的 proto 结构转成 catalog 值。
// 派生字段（CategoryID/Accent 等）交给 normalize 回填，避免两边各写一份逻辑。
func catalogFromProto(site *navkitv1.Site, categories []*navkitv1.Category) *catalog.Catalog {
	out := &catalog.Catalog{
		Site: catalog.Site{
			Title:        site.GetTitle(),
			Subtitle:     site.GetSubtitle(),
			Logo:         site.GetLogo(),
			Announcement: site.GetAnnouncement(),
			Footer:       site.GetFooter(),
		},
		Categories: make([]*catalog.Category, 0, len(categories)),
	}
	for _, cat := range categories {
		next := &catalog.Category{
			ID:          cat.GetId(),
			Name:        cat.GetName(),
			Icon:        cat.GetIcon(),
			Accent:      cat.GetAccent(),
			Description: cat.GetDescription(),
			Links:       make([]*catalog.Link, 0, len(cat.GetLinks())),
		}
		for _, link := range cat.GetLinks() {
			next.Links = append(next.Links, &catalog.Link{
				ID:     link.GetId(),
				Name:   link.GetName(),
				URL:    link.GetUrl(),
				Icon:   link.GetIcon(),
				Desc:   link.GetDesc(),
				Tags:   link.GetTags(),
				Pinned: link.GetPinned(),
			})
		}
		out.Categories = append(out.Categories, next)
	}
	return out
}

// ---------------------------------------------------------------- 映射

func siteToProto(c *catalog.Catalog) *navkitv1.Site {
	return &navkitv1.Site{
		Title:        c.Site.Title,
		Subtitle:     c.Site.Subtitle,
		Logo:         c.Site.Logo,
		Announcement: c.Site.Announcement,
		Footer:       c.Site.Footer,
	}
}

func metaToProto(c *catalog.Catalog) *navkitv1.CatalogMeta {
	return &navkitv1.CatalogMeta{
		Revision:    c.Revision(),
		ContentHash: c.ContentHash(),
		UpdatedAt:   timestamppb.New(c.UpdatedAt()),
		Source:      c.Source(),
		ConfigPath:  c.SourcePath(),
	}
}

func categoryToProto(cat *catalog.Category, withLinks bool) *navkitv1.Category {
	out := &navkitv1.Category{
		Id:          cat.ID,
		Name:        cat.Name,
		Icon:        cat.Icon,
		Accent:      cat.Accent,
		Description: cat.Description,
	}
	if !withLinks {
		return out
	}
	out.Links = make([]*navkitv1.Link, 0, len(cat.Links))
	for _, link := range cat.Links {
		out.Links = append(out.Links, linkToProto(link))
	}
	return out
}

func linkToProto(l *catalog.Link) *navkitv1.Link {
	return &navkitv1.Link{
		Id:           l.ID,
		Name:         l.Name,
		Url:          l.URL,
		Icon:         l.Icon,
		Desc:         l.Desc,
		Tags:         append([]string(nil), l.Tags...),
		Pinned:       l.Pinned,
		CategoryId:   l.CategoryID,
		CategoryName: l.CategoryName,
		Accent:       l.Accent,
	}
}
