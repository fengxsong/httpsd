package discovery

import (
	"context"
	"fmt"
	"net/url"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type Discoverer interface {
	Refresh(context.Context, url.Values) ([]*targetgroup.Group, error)
}

type Builder interface {
	AddFlags(*kingpin.Application)
	Build(log.Logger, prometheus.Registerer) (Discoverer, error)
}

var builers = make(map[string]Builder)

func Register(name string, builder Builder) error {
	if _, ok := builers[name]; ok {
		return fmt.Errorf("already registered discoverer %s", name)
	}
	builers[name] = builder
	return nil
}

func All() map[string]Builder {
	return builers
}
