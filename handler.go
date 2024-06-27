package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"

	httpdiscoverer "github.com/fengxsong/httpsd/transform/http"
	_ "github.com/fengxsong/httpsd/transform/nacos"
)

type sdHandler struct {
	discoverer *httpdiscoverer.Discovery
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

	targetgroups, err := h.discoverer.Refresh(req.Context(), q)
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

func loadConfigFile(filename string) (*httpdiscoverer.SDConfig, []byte, error) {
	cfg := &httpdiscoverer.DefaultSDConfig
	content, err := os.ReadFile(filename)
	if err != nil {
		return cfg, nil, err
	}
	err = yaml.UnmarshalStrict([]byte(content), cfg)
	if err != nil {
		return nil, nil, err
	}
	cfg.HTTPClientConfig.SetDirectory(filepath.Dir(filepath.Dir(filename)))
	return cfg, content, nil
}

func newSDHandler(cfg *httpdiscoverer.SDConfig, logger log.Logger, registerer prometheus.Registerer) (*sdHandler, error) {
	discoverer, err := httpdiscoverer.NewDiscovery(cfg, logger, nil, registerer)
	if err != nil {
		return nil, err
	}
	return &sdHandler{
		discoverer: discoverer,
		logger:     logger,
	}, nil
}
