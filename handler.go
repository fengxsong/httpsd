package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/fengxsong/httpsd/pkg/discovery"
	_ "github.com/fengxsong/httpsd/pkg/discovery/http"
	_ "github.com/fengxsong/httpsd/pkg/discovery/nacos"
)

type options struct {
	path string
	t    string
}

func (o *options) AddFlags(app *kingpin.Application) {
	app.Flag("uri.path", "path of target url").Default("/targets").StringVar(&o.path)
	app.Flag("discoverer.type", "type of discoverer").Default("http").StringVar(&o.t)
}

type sdHandler struct {
	defaultT   string
	discoverer map[string]discovery.Discoverer
	logger     log.Logger
}

func httpErrorWithLogging(w http.ResponseWriter, logger log.Logger, err string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error": "%s"}`, err)
	level.Error(logger).Log("err", err)
}

func (h *sdHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	pretty, _ := strconv.ParseBool(q.Get("pretty"))
	q.Del("pretty")

	t := q.Get("discovery")
	if t == "" {
		t = h.defaultT
	}

	discovery := h.discoverer[t]
	if discovery == nil {
		httpErrorWithLogging(w, h.logger, fmt.Sprintf("unknown discoverer %s", t), http.StatusInternalServerError)
		return
	}
	targetgroups, err := discovery.Refresh(req.Context(), q)
	if err != nil {
		httpErrorWithLogging(w, h.logger, err.Error(), http.StatusInternalServerError)
		return
	}
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err = encoder.Encode(targetgroups); err != nil {
		httpErrorWithLogging(w, h.logger, err.Error(), http.StatusInternalServerError)
	}
}

func newSDHandler(o *options, logger log.Logger, registerer prometheus.Registerer) (*sdHandler, error) {
	handler := &sdHandler{
		defaultT:   o.t,
		discoverer: map[string]discovery.Discoverer{},
		logger:     logger,
	}
	for name, builder := range discovery.All() {
		d, err := builder.Build(logger, registerer)
		if d == nil || err != nil {
			level.Info(logger).Log("msg", fmt.Sprintf("skip discoverer %s due to err: %s", name, err))
			continue
		}
		handler.discoverer[name] = d
	}
	return handler, nil
}
