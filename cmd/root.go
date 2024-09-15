package cmd

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ginuerzh/gost-plugins/ingress"
	"github.com/ginuerzh/gost-plugins/recorder"
	"github.com/ginuerzh/gost-plugins/sd"
	"github.com/spf13/cobra"
)

var (
	addr            string
	redisAddr       string
	redisDB         int
	redisExpiration time.Duration
	domain          string

	mongoURI string
	mongoDB  string
	Timeout  time.Duration

	// log flags
	logLevel  string
	logFormat string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "gost-plugins",
	Short:        "GOST plugins",
	Long:         "A set of plugins for GOST.PLUS",
	SilenceUsage: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		llevel := slog.LevelInfo
		switch strings.ToLower(logLevel) {
		case "debug":
			llevel = slog.LevelDebug
		case "info":
			llevel = slog.LevelInfo
		case "warn":
			llevel = slog.LevelWarn
		case "error":
			llevel = slog.LevelError
		}

		if logFormat == "json" {
			slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
				Level: llevel,
			})))
		} else {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: llevel,
			})))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&logLevel, "log.level", "info", "log level: debug, info, warn or error")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log.format", "json", "log format: text or json")
	rootCmd.PersistentFlags().StringVar(&addr, "addr", ":8000", "the plugin service address")

	ingressCmd := &cobra.Command{
		Use:   "ingress",
		Short: "Ingress plugin",
		Long:  "Ingress plugin for GOST.PLUS",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ingress.ListenAndServe(addr, &ingress.Options{
				RedisAddr:       redisAddr,
				RedisDB:         redisDB,
				RedisExpiration: redisExpiration,
				Domain:          domain,
			})
		},
	}
	ingressCmd.Flags().StringVar(&redisAddr, "redis.addr", "127.0.0.1:6379", "redis server address")
	ingressCmd.Flags().IntVar(&redisDB, "redis.db", 0, "redis database")
	ingressCmd.Flags().DurationVar(&redisExpiration, "redis.expiration", time.Hour, "redis key expiration")
	ingressCmd.Flags().StringVar(&domain, "domain", "gost.plus", "domain name")

	sdCmd := &cobra.Command{
		Use:   "sd",
		Short: "SD plugin",
		Long:  "Service discovery plugin for GOST.PLUS",
		RunE: func(cmd *cobra.Command, args []string) error {
			return sd.ListenAndServe(addr, &sd.Options{
				RedisAddr:       redisAddr,
				RedisDB:         redisDB,
				RedisExpiration: redisExpiration,
			})
		},
	}
	sdCmd.Flags().StringVar(&redisAddr, "redis.addr", "127.0.0.1:6379", "redis server address")
	sdCmd.Flags().IntVar(&redisDB, "redis.db", 0, "redis database")
	sdCmd.Flags().DurationVar(&redisExpiration, "redis.expiration", time.Minute, "redis key expiration")

	recorderCmd := &cobra.Command{
		Use:   "recorder",
		Short: "Recorder plugin",
		Long:  "Recorder plugin HTTP service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return recorder.ListenAndServe(addr, &recorder.Options{
				MongoURI: mongoURI,
				MongoDB:  mongoDB,
				Timeout:  Timeout,
			})
		},
	}
	recorderCmd.Flags().StringVar(&mongoURI, "mongo.uri", "mongodb://127.0.0.1:27017", "MongoDB server address")
	recorderCmd.Flags().StringVar(&mongoDB, "mongo.db", "gost", "MongoDB database")
	recorderCmd.Flags().DurationVar(&Timeout, "mongo.timeout", 10*time.Second, "MongoDB connection timeout")

	rootCmd.AddCommand(ingressCmd, sdCmd, recorderCmd)
}
