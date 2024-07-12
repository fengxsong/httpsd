package utils

import (
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

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
