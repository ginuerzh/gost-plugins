package sd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"time"

	sd_proto "github.com/go-gost/plugin/sd/proto"
	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type service struct {
	// node ID
	Node string
	// tcp/udp
	Network string
	Address string
	// 最后更新时间
	Renew int64
}

type Options struct {
	RedisAddr       string
	RedisDB         int
	RedisExpiration time.Duration
}

type server struct {
	client *redis.Client
	sd_proto.UnimplementedSDServer
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

	sd_proto.RegisterSDServer(s, srv)
	return s.Serve(ln)
}

func (s *server) Register(ctx context.Context, in *sd_proto.RegisterRequest) (*sd_proto.RegisterReply, error) {
	reply := &sd_proto.RegisterReply{}

	srv := in.GetService()
	if srv == nil || srv.Id == "" || srv.Name == "" || srv.Node == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid args")
	}

	log := slog.With("op", "register", "tunnel", srv.Name, "connector", srv.Id, "node", srv.Node, "network", srv.Network, "address", srv.Address)

	var addr net.Addr
	var err error
	switch srv.Network {
	case "udp":
		addr, err = net.ResolveUDPAddr("udp", srv.Address)
	default:
		addr, err = net.ResolveTCPAddr("tcp", srv.Address)
	}
	if err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	address := srv.Address
	if host, port, _ := net.SplitHostPort(address); host == "" {
		if peer, _ := peer.FromContext(ctx); peer != nil && peer.Addr != nil {
			host := peer.Addr.String()
			if h, _, _ := net.SplitHostPort(host); h != "" {
				host = h
			}
			address = net.JoinHostPort(host, port)
		}
	}

	sv := service{
		Node:    srv.Node,
		Network: addr.Network(),
		Address: address,
		Renew:   time.Now().Unix(),
	}
	v, err := json.Marshal(sv)
	if err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if _, err := s.client.HSet(ctx, srv.Name, srv.Id, v).Result(); err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	s.client.Expire(ctx, srv.Name, s.opts.RedisExpiration)

	log.Info(fmt.Sprintf("register tunnel=%s, connector=%s, address=%s/%s", srv.Name, srv.Id, sv.Address, sv.Network))
	reply.Ok = true

	return reply, nil
}

func (s *server) Deregister(ctx context.Context, in *sd_proto.DeregisterRequest) (*sd_proto.DeregisterReply, error) {
	reply := &sd_proto.DeregisterReply{}

	srv := in.GetService()
	if srv == nil || srv.Id == "" || srv.Name == "" {
		return reply, nil
	}

	log := slog.With("op", "deregister", "tunnel", srv.Name, "connector", srv.Id, "node", srv.Node)

	if _, err := s.client.HDel(ctx, srv.Name, srv.Id).Result(); err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}
	log.Info(fmt.Sprintf("deregister tunnel=%s, connector=%s", srv.Id, srv.Name))

	reply.Ok = true

	return reply, nil
}

func (s *server) Renew(ctx context.Context, in *sd_proto.RenewRequest) (*sd_proto.RenewReply, error) {
	reply := &sd_proto.RenewReply{}

	srv := in.GetService()
	if srv == nil || srv.Id == "" || srv.Name == "" {
		return reply, nil
	}

	log := slog.With("op", "renew", "tunnel", srv.Name, "connector", srv.Id, "node", srv.Node)

	v, err := s.client.HGet(ctx, srv.Name, srv.Id).Bytes()
	if err != nil {
		if err == redis.Nil {
			return reply, nil
		}
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	var sv service
	if err := json.Unmarshal(v, &sv); err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	/*
		// expired
		if time.Since(time.Unix(sv.Renew, 0)) > s.opts.RedisExpiration {
			return reply, nil
		}
	*/

	sv.Renew = time.Now().Unix()
	v, _ = json.Marshal(sv)

	if _, err := s.client.HSet(ctx, srv.Name, srv.Id, v).Result(); err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	s.client.Expire(ctx, srv.Name, s.opts.RedisExpiration)

	log.Info(fmt.Sprintf("renew tunnel=%s, connector=%s", srv.Name, srv.Id))
	reply.Ok = true

	return reply, nil
}

func (s *server) Get(ctx context.Context, in *sd_proto.GetServiceRequest) (*sd_proto.GetServiceReply, error) {
	reply := &sd_proto.GetServiceReply{}

	if in.Name == "" {
		return reply, nil
	}

	log := slog.With("op", "get", "tunnel", in.Name)

	m, err := s.client.HGetAll(ctx, in.Name).Result()
	if err != nil {
		log.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	var srv service
	var services []*sd_proto.Service
	for k, v := range m {
		if err := json.Unmarshal([]byte(v), &srv); err != nil {
			continue
		}
		if srv.Node == "" || srv.Address == "" {
			continue
		}
		if time.Since(time.Unix(srv.Renew, 0)) > s.opts.RedisExpiration {
			continue
		}
		services = append(services, &sd_proto.Service{
			Id:      k,
			Name:    in.Name,
			Node:    srv.Node,
			Network: srv.Network,
			Address: srv.Address,
		})
	}

	if len(services) > 1 {
		rand.Shuffle(len(services), func(i, j int) {
			services[i], services[j] = services[j], services[i]
		})
	}
	log.Debug(fmt.Sprintf("get services: %+v", services))

	reply.Services = services
	return reply, nil
}
