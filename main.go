package main

import (
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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

	"github.com/fengxsong/httpsd/pkg/discovery"
)

func main() {
	os.Exit(run())
}

func run() int {
	app := kingpin.New(filepath.Base(os.Args[0]), "")
	toolkitFlags := webflag.AddFlags(app, ":8080")
	promlogConfig := &promlog.Config{}
	flag.AddFlags(app, promlogConfig)

	for _, builder := range discovery.All() {
		builder.AddFlags(app)
	}

	o := &options{}
	o.AddFlags(app)

	app.Version(version.Print("httpsd"))
	app.HelpFlag.Short('h')
	kingpin.MustParse(app.Parse(os.Args[1:]))
	logger := promlog.New(promlogConfig)

	reg := prometheus.NewRegistry()

	handler, err := newSDHandler(o, logger, reg)
	if err != nil {
		level.Error(logger).Log("err", err)
		return 1
	}

	http.Handle(o.path, handler)
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
