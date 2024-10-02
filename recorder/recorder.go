package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Options struct {
	MongoURI string
	MongoDB  string
	LokiURL  string
	LokiID   string
	Timeout  time.Duration
}

type server struct {
	lokiClient  *http.Client
	mongoClient *mongo.Client
	opts        *Options
}

func ListenAndServe(addr string, opts *Options) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info(fmt.Sprintf("server listening on %v", ln.Addr()))

	srv := &server{
		opts: opts,
	}

	if opts.MongoURI != "" {
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

		srv.mongoClient = client
	}

	if opts.LokiURL != "" {
		srv.lokiClient = &http.Client{
			Timeout: opts.Timeout,
		}
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

	if s.mongoClient != nil {
		col := s.mongoClient.Database(s.opts.MongoDB).Collection("recorders")
		if _, err := col.InsertOne(r.Context(), o); err != nil {
			slog.Error(fmt.Sprintf("mongo %s: %v", o.SID, err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	if err := s.pushLoki(&o); err != nil {
		slog.Error(fmt.Sprintf("loki %s: %v", o.SID, err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *server) pushLoki(o *HandlerRecorderObject) error {
	if s.lokiClient == nil || o == nil {
		return nil
	}

	md := lokiMetadata{
		ClientID: o.ClientID,
		ClientIP: o.ClientIP,
		Node:     o.Node,
		SID:      o.SID,
		Route:    o.Route,
		Duration: strconv.FormatInt(o.Duration.Nanoseconds(), 10),
		Ts:       o.Time,
	}
	if o.Err != "" {
		md.Error = "true"
	}

	clientIP := o.ClientIP
	if clientIP == "" {
		clientIP = "-"
	}

	host := o.Host
	if host == "" {
		host = "-"
	}

	msg := &bytes.Buffer{}
	fmt.Fprintf(msg, "%s %s", clientIP, host)

	if o.TLS != nil {
		version := o.TLS.Version
		if version == "" {
			version = "-"
		}
		proto := o.TLS.Proto
		if proto == "" {
			proto = "-"
		}

		fmt.Fprintf(msg, " %s %s", version, proto)
		md.TLSCipherSuite = o.TLS.CipherSuite
		md.TLSVersion = o.TLS.Version
	}
	if o.HTTP != nil {
		fmt.Fprintf(msg, " %s %s %s %s %d %d %d",
			o.HTTP.Method, o.HTTP.Host, o.HTTP.URI, o.HTTP.Proto, o.HTTP.StatusCode, o.HTTP.Request.ContentLength, o.HTTP.Response.ContentLength)

		buf := bytes.Buffer{}
		if h := o.HTTP.Request.Header; h != nil {
			o.HTTP.Request.Header.Write(&buf)
			md.HTTPRequestHeader = buf.String()
		}
		if h := o.HTTP.Response.Header; h != nil {
			buf.Reset()
			o.HTTP.Response.Header.Write(&buf)
			md.HTTPResponseHeader = buf.String()
		}
	}
	if o.DNS != nil {
		fmt.Fprintf(msg, " %s %s %s", strings.TrimSuffix(o.DNS.Name, "."), o.DNS.Class, o.DNS.Type)
		md.DNSQuestion = o.DNS.Question
		md.DNSAnswer = o.DNS.Answer
		md.DNSCached = fmt.Sprintf("%v", o.DNS.Cached)
	}

	fmt.Fprintf(msg, " %v", o.Duration)
	if o.Err != "" {
		fmt.Fprintf(msg, " %s", o.Err)
	}

	body := lokiBody{
		Streams: []lokiStream{
			{
				Stream: lokiStreamObject{Service: o.Service},
				Values: [][]interface{}{
					{
						strconv.FormatInt(time.Now().UnixNano(), 10),
						msg,
						md,
					},
				},
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, s.opts.LokiURL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Scope-OrgId", s.opts.LokiID)

	resp, err := s.lokiClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 300 {
		return errors.New(resp.Status)
	}

	return nil
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

type TLSRecorderObject struct {
	ServerName        string `json:"serverName"`
	CipherSuite       string `json:"cipherSuite"`
	CompressionMethod uint8  `json:"compressionMethod"`
	Proto             string `json:"proto"`
	Version           string `json:"version"`
	ClientHello       string `json:"clientHello"`
	ServerHello       string `json:"serverHello"`
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
	Proto      string              `json:"proto"`
	ClientIP   string              `json:"clientIP"`
	ClientID   string              `json:"clientID,omitempty"`
	HTTP       *HTTPRecorderObject `json:"http,omitempty"`
	DNS        *DNSRecorderObject  `json:"dns,omitempty"`
	TLS        *TLSRecorderObject  `json:"tls,omitempty"`
	Route      string              `json:"route,omitempty"`
	Err        string              `json:"err,omitempty"`
	SID        string              `json:"sid"`
	Duration   time.Duration       `json:"duration"`
	Time       time.Time           `json:"time"`
}

type lokiBody struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream lokiStreamObject `json:"stream"`
	Values [][]interface{}  `json:"values"`
}

type lokiStreamObject struct {
	Service string `json:"service_name"`
}

type lokiMetadata struct {
	Node               string    `json:"node,omitempty"`
	ClientID           string    `json:"client_id,omitempty"`
	ClientIP           string    `json:"client_ip"`
	SID                string    `json:"sid"`
	HTTPRequestHeader  string    `json:"http_request_header,omitempty"`
	HTTPResponseHeader string    `json:"http_response_header,omitempty"`
	TLSCipherSuite     string    `json:"tls_cipher_suite,omitempty"`
	TLSVersion         string    `json:"tls_version,omitempty"`
	DNSQuestion        string    `json:"dns_question,omitempty"`
	DNSAnswer          string    `json:"dns_answer,omitempty"`
	DNSCached          string    `json:"dns_cached,omitempty"`
	Route              string    `json:"route,omitempty"`
	Error              string    `json:"error,omitempty"`
	Duration           string    `json:"duration"`
	Ts                 time.Time `json:"ts"`
}
