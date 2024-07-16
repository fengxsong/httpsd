package asitis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/fengxsong/httpsd/pkg/transformer"
)

const name = "asitis"

type asitis struct {
	t *transformer.Template
}

func (asitis) Name() string { return name }

func (asitis) SampleConfig() transformer.Config {
	return &transformer.Template{}
}

func (a *asitis) Init(v transformer.Config) error {
	t, ok := v.(*transformer.Template)
	if !ok {
		return fmt.Errorf("unexpected config: %T", v)
	}
	a.t = t
	return nil
}

// TargetURL validate base url and query values here
func (asitis) TargetURL(base string, q url.Values) (string, error) {
	parsedURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	qs := parsedURL.Query()
	for k, v := range q {
		for _, vv := range v {
			qs.Add(k, vv)
		}
	}
	parsedURL.RawQuery = qs.Encode()
	return parsedURL.String(), nil
}

func (asitis) HTTPMethod() string { return http.MethodGet }

// Transform unmarshal response body into array of targetgroup.Group
func (a *asitis) Transform(_ context.Context, b []byte) ([]*targetgroup.Group, error) {
	if a.t == nil || (a.t.GoTemplate == "" && a.t.JSONPath == "") {
		var targetGroups []*targetgroup.Group
		err := json.Unmarshal(b, &targetGroups)
		return targetGroups, err
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	parsed, err := a.t.Execute(data)
	if err != nil {
		return nil, err
	}
	var tgs []*targetgroup.Group
	err = json.Unmarshal(parsed, &tgs)
	return tgs, err
}

func init() {
	if err := transformer.Register(&asitis{}); err != nil {
		panic(err)
	}
}
