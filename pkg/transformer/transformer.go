package transformer

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"

	"github.com/Masterminds/sprig/v3"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"k8s.io/client-go/util/jsonpath"
)

var defaultTpl *template.Template

// TODO: add more template functions like https://github.com/helm/helm/blob/main/pkg/engine/engine.go#L193
func init() {
	defaultTpl = template.New("defaultTpl").Option("missingkey=default").Funcs(sprig.FuncMap())
}

type Template struct {
	GoTemplate string `yaml:"gotemplate,omitempty"`
	JSONPath   string `yaml:"jsonpath,omitempty"`
}

func (t *Template) Execute(data any) ([]byte, error) {
	out := bytes.NewBuffer([]byte{})
	var err error
	if t.GoTemplate != "" {
		tpl, err := defaultTpl.Clone()
		if err != nil {
			return nil, err
		}
		tpl, err = tpl.Parse(t.GoTemplate)
		if err != nil {
			return nil, err
		}
		err = tpl.Execute(out, data)
	} else if t.JSONPath != "" {
		jpath := jsonpath.New("t")
		if err := jpath.Parse(t.JSONPath); err != nil {
			return nil, err
		}
		err = jpath.Execute(out, data)
	}
	return out.Bytes(), err
}

type Config any

type Transformer interface {
	Name() string
	SampleConfig() Config
	Init(Config) error
	TargetURL(string, url.Values) (string, error)
	HTTPMethod() string
	Transform(context.Context, []byte) ([]*targetgroup.Group, error)
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
