package transform

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Transformer interface {
	Name() string
	TargetURL(string, url.Values) (string, error)
	HTTPMethod() string
	Transform([]byte) ([]*targetgroup.Group, error)
}

var transformers = map[string]Transformer{}

var formalizeReplacer = strings.NewReplacer("-", "_", ".", "_")

func FormalizeLabelName(s string) string {
	return formalizeReplacer.Replace(s)
}

func Grouping(tgs []*targetgroup.Group) []*targetgroup.Group {
	m := make(map[model.Fingerprint]*targetgroup.Group)
	var fpset model.Fingerprints
	for i := range tgs {
		fingerprint := tgs[i].Labels.Fingerprint()
		if v, ok := m[fingerprint]; ok {
			v.Targets = append(v.Targets, tgs[i].Targets...)
		} else {
			m[fingerprint] = tgs[i]
			fpset = append(fpset, fingerprint)
		}
	}
	// sorting
	sort.Sort(fpset)
	ret := make([]*targetgroup.Group, 0, len(m))
	for _, fingerprint := range fpset {
		ret = append(ret, m[fingerprint])
	}
	return ret
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
