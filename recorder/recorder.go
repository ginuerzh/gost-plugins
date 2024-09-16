package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Options struct {
	MongoURI string
	MongoDB  string
	Timeout  time.Duration
}

type server struct {
	client *mongo.Client
	opts   *Options
}

func ListenAndServe(addr string, opts *Options) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("server listening on %v", ln.Addr()))

	ctx := context.Background()

	if opts.Timeout > 0 {
		ctx2, cancel := context.WithTimeout(context.Background(), opts.Timeout)
		ctx = ctx2
		defer cancel()
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(opts.MongoURI))
	if err != nil {
		return err
	}
	defer client.Disconnect(ctx)

	srv := &server{
		client: client,
		opts:   opts,
	}

	mux := http.NewServeMux()
	mux.Handle("/", srv)

	s := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s.Serve(ln)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if slog.Default().Enabled(r.Context(), slog.LevelDebug) {
		dump, err := httputil.DumpRequest(r, true)
		if err != nil {
			slog.Error(fmt.Sprintf("%v", err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		slog.Debug(string(dump))
	}

	o := HandlerRecorderObject{}
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		slog.Error(fmt.Sprintf("%v", err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	col := s.client.Database(s.opts.MongoDB).Collection("recorders")
	if _, err := col.InsertOne(r.Context(), o); err != nil {
		slog.Error(fmt.Sprintf("save record %s: %v", o.SID, err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

type HTTPRequestRecorderObject struct {
	ContentLength int64       `json:"contentLength"`
	Header        http.Header `json:"header"`
	Body          []byte      `json:"body"`
}

type HTTPResponseRecorderObject struct {
	ContentLength int64       `json:"contentLength"`
	Header        http.Header `json:"header"`
	Body          []byte      `json:"body"`
}

type HTTPRecorderObject struct {
	Host       string                     `json:"host"`
	Method     string                     `json:"method"`
	Proto      string                     `json:"proto"`
	Scheme     string                     `json:"scheme"`
	URI        string                     `json:"uri"`
	StatusCode int                        `json:"statusCode"`
	Request    HTTPRequestRecorderObject  `json:"request"`
	Response   HTTPResponseRecorderObject `json:"response"`
}

type DNSRecorderObject struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Class    string `json:"class"`
	Type     string `json:"type"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Cached   bool   `json:"cached"`
}

type HandlerRecorderObject struct {
	Node       string              `json:"node,omitempty"`
	Service    string              `json:"service"`
	Network    string              `json:"network"`
	RemoteAddr string              `json:"remote"`
	LocalAddr  string              `json:"local"`
	Host       string              `json:"host"`
	ClientIP   string              `json:"clientIP"`
	ClientID   string              `json:"clientID,omitempty"`
	HTTP       *HTTPRecorderObject `json:"http,omitempty"`
	DNS        *DNSRecorderObject  `json:"dns,omitempty"`
	Err        string              `json:"err,omitempty"`
	Duration   time.Duration       `json:"duration"`
	SID        string              `json:"sid"`
	Time       time.Time           `json:"time"`
}
