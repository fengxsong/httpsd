package transform

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Transformer interface {
	Name() string
	TargetURL(string, url.Values) string
	HTTPMethod() string
	Transform([]byte) ([]*targetgroup.Group, error)
}

var transformers = map[string]Transformer{}

var formalizeReplacer = strings.NewReplacer("-", "_", ".", "_")

func FormalizeKeyName(s string) string {
	return formalizeReplacer.Replace(s)
}

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
