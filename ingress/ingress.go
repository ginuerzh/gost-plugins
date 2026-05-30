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
	RedisUsername   string
	RedisPassword   string
	RedisExpiration time.Duration
	Domains         []string
	MinDomain       int
}

type server struct {
	client *redis.Client
	ingress_proto.UnimplementedIngressServer
	opts *Options
}

func ListenAndServe(addr string, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("server listening on %v", ln.Addr()))

	s := grpc.NewServer()
	srv := &server{
		client: redis.NewClient(&redis.Options{
			Addr:     opts.RedisAddr,
			DB:       opts.RedisDB,
			Username: opts.RedisUsername,
			Password: opts.RedisPassword,
		}),
		opts: opts,
	}
	defer srv.client.Close()

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
	if len(host) < s.opts.MinDomain {
		return reply, nil
	}

	ok, err := s.client.SetNX(ctx, host, tid.String(), s.opts.RedisExpiration).Result()
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	if ok {
		slog.Debug(fmt.Sprintf("set: %s -> %s -> %s", in.Host, host, tid.String()))
		reply.Ok = true
	} else {
		if v, err := s.client.Get(ctx, host).Result(); err != nil {
			slog.Error(fmt.Sprintf("get: %v", err))
		} else if v == in.Endpoint {
			reply.Ok = true
		}
	}
	return reply, nil
}

func (s *server) GetRule(ctx context.Context, in *ingress_proto.GetRuleRequest) (*ingress_proto.GetRuleReply, error) {
	host := in.Host
	if v, _, _ := net.SplitHostPort(in.Host); v != "" {
		host = v
	}

	reply := &ingress_proto.GetRuleReply{}

	key := host
	for _, domain := range s.opts.Domains {
		if strings.HasSuffix(host, domain) {
			if n := strings.IndexByte(host, '.'); n > 0 {
				key = host[:n]
			}
		}
	}
	if len(key) >= s.opts.MinDomain {
		var err error
		reply.Endpoint, err = s.client.Get(ctx, key).Result()
		if err != nil {
			slog.Error(fmt.Sprintf("get: %v", err))
		}
	}

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
	uid, err := uuid.Parse(s)
	if err != nil {
		slog.Warn(fmt.Sprintf("parse tunnel ID: %v", err))
		return
	}

	if private {
		return relay.NewPrivateTunnelID(uid[:])
	}
	return relay.NewTunnelID(uid[:])
}
