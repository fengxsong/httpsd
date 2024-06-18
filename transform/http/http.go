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
	"strings"
	"time"

	"github.com/fengxsong/httpsd/transform"
	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	// DefaultSDConfig is the default HTTP SD configuration.
	DefaultSDConfig = SDConfig{
		Timeout:          model.Duration(60 * time.Second),
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
	userAgent        = fmt.Sprintf("HTTPSDTransformer/%s", version.Version)
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
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
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

const httpSDURLLabel = model.MetaLabelPrefix + "url"

// Discovery provides service discovery functionality based
// on HTTP endpoints that return target groups in JSON format.
type Discovery struct {
	url          string
	client       *http.Client
	tgLastLength int
	metrics      *httpMetrics
	logger       log.Logger
}

// NewDiscovery returns a new HTTP discovery for the given config.
func NewDiscovery(conf *SDConfig, logger log.Logger, clientOpts []config.HTTPClientOption, registerer prometheus.Registerer) (*Discovery, error) {
	m := newDiscovererMetrics(registerer)
	m.Register()

	if logger == nil {
		logger = log.NewNopLogger()
	}

	client, err := config.NewClientFromConfig(conf.HTTPClientConfig, "http", clientOpts...)
	if err != nil {
		return nil, err
	}
	client.Timeout = time.Duration(conf.Timeout)

	d := &Discovery{
		url:     conf.URL,
		client:  client,
		metrics: m.(*httpMetrics),
		logger:  logger,
	}

	return d, nil
}

func (d *Discovery) Refresh(ctx context.Context, t transform.Transformer, q url.Values) ([]*targetgroup.Group, error) {
	req, err := http.NewRequest(t.HTTPMethod(), t.TargetURL(d.url, q), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req.WithContext(ctx))
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

	targetGroups, err := t.Transform(b)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, err
	}

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
		tg.Labels[httpSDURLLabel] = model.LabelValue(d.url)
	}

	// Generate empty updates for sources that disappeared.
	l := len(targetGroups)
	for i := l; i < d.tgLastLength; i++ {
		targetGroups = append(targetGroups, &targetgroup.Group{Source: urlSource(d.url, i)})
	}
	d.tgLastLength = l

	return targetGroups, nil
}

// urlSource returns a source ID for the i-th target group per URL.
func urlSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}