package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	ingress_proto "github.com/go-gost/plugin/ingress/proto"
	"github.com/go-gost/relay"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

type Options struct {
	RedisAddr       string
	RedisDB         int
	RedisExpiration time.Duration
	Domain          string
}

type server struct {
	client *redis.Client
	ingress_proto.UnimplementedIngressServer
	opts *Options
}

func ListenAndServe(addr string, opts *Options) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("server listening on %v", ln.Addr()))

	s := grpc.NewServer()
	srv := &server{
		client: redis.NewClient(&redis.Options{
			Addr: opts.RedisAddr,
			DB:   opts.RedisDB,
		}),
		opts: opts,
	}

	ingress_proto.RegisterIngressServer(s, srv)
	return s.Serve(ln)
}

func (s *server) SetRule(ctx context.Context, in *ingress_proto.SetRuleRequest) (*ingress_proto.SetRuleReply, error) {
	reply := &ingress_proto.SetRuleReply{}

	tid := parseTunnelID(in.Endpoint)
	if in.Host == "" || tid.IsZero() {
		return reply, nil
	}

	host := in.Host
	if v, _, _ := net.SplitHostPort(in.Host); v != "" {
		host = v
	}

	s.client.SetNX(ctx, host, tid.String(), s.opts.RedisExpiration)
	slog.Debug(fmt.Sprintf("set: %s -> %s -> %s", in.Host, host, tid.String()))
	return reply, nil
}

func (s *server) GetRule(ctx context.Context, in *ingress_proto.GetRuleRequest) (*ingress_proto.GetRuleReply, error) {
	host := in.Host
	if v, _, _ := net.SplitHostPort(in.Host); v != "" {
		host = v
	}

	reply := &ingress_proto.GetRuleReply{}

	key := host
	if strings.HasSuffix(host, s.opts.Domain) {
		if n := strings.IndexByte(host, '.'); n > 0 {
			key = host[:n]
		}
	}
	reply.Endpoint, _ = s.client.Get(ctx, key).Result()

	slog.Debug(fmt.Sprintf("ingress: %s -> %s -> %s", in.Host, key, reply.Endpoint))
	return reply, nil
}

func parseTunnelID(s string) (tid relay.TunnelID) {
	if s == "" {
		return
	}
	private := false
	if s[0] == '$' {
		private = true
		s = s[1:]
	}
	uuid, _ := uuid.Parse(s)

	if private {
		return relay.NewPrivateTunnelID(uuid[:])
	}
	return relay.NewTunnelID(uuid[:])
}
