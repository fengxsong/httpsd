// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"gopkg.in/yaml.v2"

	"github.com/fengxsong/httpsd/pkg/discovery"
	"github.com/fengxsong/httpsd/pkg/transformer"
	"github.com/fengxsong/httpsd/pkg/utils"
)

var (
	// DefaultSDConfig is the default HTTP SD configuration.
	DefaultSDConfig = SDConfig{
		Timeout:          model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	userAgent        = fmt.Sprintf("HTTPServiceDiscoverer/%s", version.Version)
	matchContentType = regexp.MustCompile(`^(?i:application\/json(;\s*charset=("utf-8"|utf-8))?)$`)
)

// SDConfig is the configuration for HTTP based discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	Timeout          model.Duration          `yaml:"timeout,omitempty"`
	URL              string                  `yaml:"url"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	return unmarshal((*plain)(c))
}

func (c *SDConfig) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("URL is missing")
	}
	parsedURL, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL scheme must be 'http' or 'https'")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("host is missing in URL")
	}
	return c.HTTPClientConfig.Validate()
}

type options struct {
	configPath string
	url        string
	username   string
	password   string
}

func (o *options) AddFlags(app *kingpin.Application) {
	app.Flag("http.config", "path of config file").Default("").StringVar(&o.configPath)
	app.Flag("http.url", "url to fetch and convert into targetgroups").Default("").StringVar(&o.url)
	app.Flag("http.basic-auth.username", "username for basic HTTP authentication").Short('u').Default("").StringVar(&o.username)
	app.Flag("http.basic-auth.password", "password for basic HTTP authentication").Short('p').Default("").StringVar(&o.password)
}

func (o *options) Build(logger log.Logger, registerer prometheus.Registerer) (discovery.Discoverer, error) {
	if o.configPath != "" {
		content, err := os.ReadFile(o.configPath)
		if err != nil {
			return nil, err
		}
		if err = yaml.UnmarshalStrict(content, &DefaultSDConfig); err != nil {
			return nil, err
		}
		DefaultSDConfig.HTTPClientConfig.SetDirectory(filepath.Dir(filepath.Dir(o.configPath)))
	} else {
		DefaultSDConfig.URL = o.url
		if o.username != "" && o.password != "" {
			DefaultSDConfig.HTTPClientConfig.BasicAuth = &config.BasicAuth{
				Username: o.username,
				Password: config.Secret(o.password),
			}
		}
	}
	if err := DefaultSDConfig.Validate(); err != nil {
		return nil, err
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	m := newDiscovererMetrics(registerer)
	m.Register()

	client, err := config.NewClientFromConfig(DefaultSDConfig.HTTPClientConfig, "http", config.WithUserAgent(userAgent))
	if err != nil {
		return nil, err
	}
	client.Timeout = time.Duration(DefaultSDConfig.Timeout)

	d := &Discovery{
		url:     DefaultSDConfig.URL,
		client:  client,
		metrics: m.(*httpMetrics),
		logger:  logger,
	}

	return d, nil
}

// Discovery provides service discovery functionality based
// on HTTP endpoints that return target groups in JSON format.
type Discovery struct {
	url     string
	client  *http.Client
	metrics *httpMetrics
	logger  log.Logger
}

func (d *Discovery) Refresh(ctx context.Context, q url.Values) ([]*targetgroup.Group, error) {
	start := time.Now()
	t := q.Get("transformer")
	if t == "" {
		t = "asitis"
	}
	tr := transformer.Get(t)
	if tr == nil {
		return nil, fmt.Errorf("unknown transformer %s", t)
	}
	targetUrl, err := tr.TargetURL(d.url, q)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(tr.HTTPMethod(), targetUrl, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req.WithContext(ctx))
	d.metrics.discoverDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		d.metrics.failuresCount.Inc()
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if !matchContentType.MatchString(strings.TrimSpace(resp.Header.Get("Content-Type"))) {
		d.metrics.failuresCount.Inc()
		return nil, fmt.Errorf("unsupported content type %q", resp.Header.Get("Content-Type"))
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}

	targetGroups, err := tr.Transform(b)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}

	targetGroups = utils.Grouping(targetGroups)
	for i, tg := range targetGroups {
		if tg == nil {
			d.metrics.failuresCount.Inc()
			err = errors.New("nil target group item found")
			return nil, err
		}

		tg.Source = urlSource(d.url, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
	}

	return targetGroups, nil
}

// urlSource returns a source ID for the i-th target group per URL.
func urlSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}

func init() {
	discovery.Register("http", &options{})
}
