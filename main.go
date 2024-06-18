package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

func main() {
	os.Exit(run())
}

func run() int {
	toolkitFlags := webflag.AddFlags(kingpin.CommandLine, ":8080")
	configF := kingpin.Flag("config.file", "path of config file").Default("httpsd.yaml").String()
	targetURL := kingpin.Flag("target.url", "url to fetch targetgroups").Default("").String()
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("httpsd"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	cfg, _, err := loadConfigFile(*configF)
	if os.IsNotExist(err) {
		level.Info(logger).Log("msg", "using default http client config")
		if *targetURL == "" {
			level.Error(logger).Log("msg", "target.url missing")
			return 1
		}
		cfg.URL = *targetURL
	} else if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}

	reg := prometheus.NewRegistry()

	handler, err := newSDHandler(cfg, logger, reg)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}

	http.Handle("/sd", handler)
	http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Healthy"))
	})
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	srv := &http.Server{}
	srvc := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}
}
