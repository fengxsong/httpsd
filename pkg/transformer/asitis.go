package transformer

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type asitis struct{}

func (asitis) Name() string { return "asitis" }

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
func (asitis) Transform(b []byte) ([]*targetgroup.Group, error) {
	var targetGroups []*targetgroup.Group
	err := json.Unmarshal(b, &targetGroups)
	return targetGroups, err
}

func init() {
	if err := Register(asitis{}); err != nil {
		panic(err)
	}
}
