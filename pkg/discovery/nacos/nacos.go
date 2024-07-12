package nacos

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/regexp"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacoslogger "github.com/nacos-group/nacos-sdk-go/v2/common/logger"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/fengxsong/httpsd/pkg/discovery"
	"github.com/fengxsong/httpsd/pkg/utils"
)

const name = "nacos"

type options struct {
	ipAddresses []string
	port        uint64
	username    string
	password    string
	namespace   string
	exclude     []string
	include     []string
	interval    time.Duration
	quiet       bool
}

func (o *options) AddFlags(app *kingpin.Application) {
	app.Flag("nacos.address", "addresses of nacos server").Default("").StringsVar(&o.ipAddresses)
	app.Flag("nacos.port", "port of nacos server").Default("8848").Uint64Var(&o.port)
	app.Flag("nacos.username", "username for basicauth").Default("").StringVar(&o.username)
	app.Flag("nacos.password", "password for basicauth").Default("").StringVar(&o.password)
	app.Flag("nacos.quiet", "discard noisy logging of nacos sdk").Default("true").BoolVar(&o.quiet)
	app.Flag("nacos.namespace", "namespace id of services").Default("").StringVar(&o.namespace)
	app.Flag("nacos.exclude", "pattern or regexp of serviceName to be excluded").Default("").StringsVar(&o.exclude)
	app.Flag("nacos.interval", "sync interval").Default("60s").DurationVar(&o.interval)
	app.Flag("nacos.include", "pattern or regexp of serviceName to be included").Default("").StringsVar(&o.include)
}

func (o *options) Build(logger log.Logger, registerer prometheus.Registerer) (discovery.Discoverer, error) {
	l := log.With(logger, "discoverer", name)
	if o.quiet {
		l = log.NewNopLogger()
		nacoslogger.SetLogger(&wrapLogger{l})
	}
	sc := []constant.ServerConfig{}
	for _, addr := range o.ipAddresses {
		sc = append(sc, *constant.NewServerConfig(addr, o.port))
	}
	cc := *constant.NewClientConfig(constant.WithUsername(o.username), constant.WithPassword(o.password), constant.WithNamespaceId(o.namespace))
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return nil, err
	}
	var exclude, include []*regexp.Regexp
	for _, pattern := range o.exclude {
		exclude = append(exclude, regexp.MustCompile(pattern))
	}
	for _, pattern := range o.include {
		include = append(include, regexp.MustCompile(pattern))
	}
	return &impl{
		o:       o,
		exclude: exclude,
		include: include,
		client:  client,
		cache:   expirable.NewLRU[string, any](1<<10, nil, 5*time.Minute),
	}, nil
}

type impl struct {
	o       *options
	exclude []*regexp.Regexp
	include []*regexp.Regexp
	cache   *expirable.LRU[string, any]
	client  naming_client.INamingClient
}

func (impl *impl) listAllServices(_ context.Context) ([]string, error) {
	var services []string
	pageno := 1
	for {
		sl, err := impl.client.GetAllServicesInfo(vo.GetAllServiceInfoParam{NameSpace: impl.o.namespace, PageNo: uint32(pageno), PageSize: 256})
		if err != nil {
			return nil, err
		}
		services = append(services, sl.Doms...)
		if sl.Count == int64(len(services)) {
			break
		}
		pageno++
	}
	return services, nil
}

// filter out if return value is true
func (impl *impl) filter(s string) bool {
	for _, reg := range impl.exclude {
		if reg.MatchString(s) {
			return true
		}
	}
	for _, reg := range impl.include {
		if !reg.MatchString(s) {
			return true
		}
	}
	return false
}

func (impl *impl) listServices(ctx context.Context) ([]string, error) {
	all, err := impl.listAllServices(ctx)
	if err != nil {
		return nil, err
	}
	var ret []string
	for _, s := range all {
		if impl.filter(s) {
			continue
		}
		ret = append(ret, s)
	}
	return ret, nil
}

func (impl *impl) getTargetgroupForService(s string) ([]*targetgroup.Group, error) {
	_, err := impl.client.GetService(vo.GetServiceParam{ServiceName: s})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (impl *impl) sync(ctx context.Context) error {
	ticker := time.NewTicker(impl.o.interval)
	for {
		select {
		case <-ticker.C:
			services, err := impl.listServices(ctx)
			if err != nil {
				return err
			}
			for _, s := range services {
				impl.cache.Add(s, struct{}{})
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (impl *impl) Refresh(ctx context.Context, q url.Values) ([]*targetgroup.Group, error) {
	return nil, nil
}

func labelName(k string, args ...string) model.LabelName {
	placeholder := ""
	if len(args) > 0 {
		placeholder = args[0]
	}
	return model.LabelName(fmt.Sprintf("%s%s%s_%s", model.MetaLabelPrefix, name, placeholder, utils.FormalizeLabelName(k)))
}

type wrapLogger struct {
	log.Logger
}

func (l *wrapLogger) Info(args ...interface{}) {
	level.Info(l.Logger).Log(args...)
}
func (l *wrapLogger) Warn(args ...interface{}) {
	level.Warn(l.Logger).Log(args...)
}
func (l *wrapLogger) Error(args ...interface{}) {
	level.Error(l.Logger).Log(args...)
}
func (l *wrapLogger) Debug(args ...interface{}) {
	level.Debug(l.Logger).Log(args...)
}
func (l *wrapLogger) Infof(template string, args ...interface{}) {
	level.Info(l.Logger).Log(fmt.Sprintf(template, args...))
}
func (l *wrapLogger) Warnf(template string, args ...interface{}) {
	level.Warn(l.Logger).Log(fmt.Sprintf(template, args...))
}
func (l *wrapLogger) Errorf(template string, args ...interface{}) {
	level.Error(l.Logger).Log(fmt.Sprintf(template, args...))
}
func (l *wrapLogger) Debugf(template string, args ...interface{}) {
	level.Debug(l.Logger).Log(fmt.Sprintf(template, args...))
}

func init() {
	discovery.Register(name, &options{})
}
