package transformer

import (
	"fmt"
	"net/url"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Transformer interface {
	Name() string
	TargetURL(string, url.Values) (string, error)
	HTTPMethod() string
	Transform([]byte) ([]*targetgroup.Group, error)
}

var transformers = map[string]Transformer{}

func Register(t Transformer) error {
	if _, ok := transformers[t.Name()]; ok {
		return fmt.Errorf("already registered transformer %s", t.Name())
	}
	transformers[t.Name()] = t
	return nil
}

func Get(name string) Transformer {
	return transformers[name]
}
