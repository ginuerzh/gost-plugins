package limiter

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	limiter_proto "github.com/go-gost/plugin/limiter/traffic/proto"
	"google.golang.org/grpc"
)

const (
	DefaultLimitIn  = 1048576
	DefaultLimitOut = 1048576
)

type Options struct {
	LimitIn  int
	LimitOut int
}

type server struct {
	limiter_proto.UnimplementedLimiterServer
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
		opts: opts,
	}

	limiter_proto.RegisterLimiterServer(s, srv)
	return s.Serve(ln)
}

func (s *server) Limit(ctx context.Context, req *limiter_proto.LimitRequest) (*limiter_proto.LimitReply, error) {
	limitIn := s.opts.LimitIn
	if limitIn <= 0 {
		limitIn = DefaultLimitIn
	}
	limitOut := s.opts.LimitOut
	if limitOut <= 0 {
		limitOut = DefaultLimitOut
	}

	slog.Debug(fmt.Sprintf("limit: %s in: %d, out: %d", req.Client, limitIn, limitOut))

	return &limiter_proto.LimitReply{
		In:  int64(limitIn),
		Out: int64(limitOut),
	}, nil
}
