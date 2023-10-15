package main

import (
	"context"
	"flag"
	"log"
	"net"
	"strings"
	"time"

	ingress_proto "github.com/go-gost/plugin/ingress/proto"
	"github.com/go-gost/relay"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"google.golang.org/grpc"
)

var (
	addr            = flag.String("addr", ":8000", "The server addr")
	redisAddr       = flag.String("redis.addr", "127.0.0.1:6379", "redis addr")
	redisDB         = flag.Int("redis.db", 0, "redis db")
	redisExpiration = flag.Duration("redis.expiration", time.Hour, "redis key expiration")
	domain          = flag.String("domain", "", "domain name e.g. gost.plus")
)

type server struct {
	client *redis.Client
	ingress_proto.UnimplementedIngressServer
}

func (s *server) Set(ctx context.Context, in *ingress_proto.SetRequest) (*ingress_proto.SetReply, error) {
	reply := &ingress_proto.SetReply{}

	tid := parseTunnelID(in.Endpoint)
	if in.Host == "" || tid.IsZero() {
		return reply, nil
	}

	host := in.Host
	if v, _, _ := net.SplitHostPort(in.Host); v != "" {
		host = v
	}

	s.client.SetNX(ctx, host, tid.String(), *redisExpiration)
	log.Printf("set: %s -> %s -> %s", in.Host, host, tid.String())
	return reply, nil
}

func (s *server) Get(ctx context.Context, in *ingress_proto.GetRequest) (*ingress_proto.GetReply, error) {
	host := in.Host
	if v, _, _ := net.SplitHostPort(in.Host); v != "" {
		host = v
	}

	reply := &ingress_proto.GetReply{}

	var key string
	if strings.HasSuffix(host, *domain) {
		if n := strings.IndexByte(host, '.'); n > 0 {
			key = host[:n]
		}
		reply.Endpoint, _ = s.client.Get(ctx, key).Result()
	}

	log.Printf("ingress: %s -> %s -> %s", in.Host, key, reply.Endpoint)
	return reply, nil
}

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	srv := &server{
		client: redis.NewClient(&redis.Options{
			Addr: *redisAddr,
			DB:   *redisDB,
		}),
	}
	ingress_proto.RegisterIngressServer(s, srv)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
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
