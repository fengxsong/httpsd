package nacos

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/regexp"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacoslogger "github.com/nacos-group/nacos-sdk-go/v2/common/logger"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"golang.org/x/sync/errgroup"

	"github.com/fengxsong/httpsd/pkg/discovery"
	"github.com/fengxsong/httpsd/pkg/transformer/nacos"
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
	app.Flag("nacos.include", "pattern or regexp of serviceName to be included").Default("").StringsVar(&o.include)
	app.Flag("nacos.interval", "sync interval").Default("60s").DurationVar(&o.interval)
}

func (o *options) Build(logger log.Logger, registerer prometheus.Registerer) (discovery.Discoverer, error) {
	l := log.With(logger, "sdk", name)
	if o.quiet {
		l = log.NewNopLogger()
	}
	nacoslogger.SetLogger(&wrapLogger{l})

	sc := []constant.ServerConfig{}
	for _, addr := range o.ipAddresses {
		sc = append(sc, *constant.NewServerConfig(addr, o.port))
	}
	if o.namespace == "" {
		level.Warn(logger).Log("msg", "--nacos.namespace is missing, fallback to 'public' namespace")
	}
	cc := constant.NewClientConfig(constant.WithUsername(o.username), constant.WithPassword(o.password), constant.WithNamespaceId(o.namespace))
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return nil, err
	}
	var exclude, include []*regexp.Regexp
	for _, pattern := range o.exclude {
		if pattern != "" {
			exclude = append(exclude, regexp.MustCompile(pattern))
		}
	}
	for _, pattern := range o.include {
		if pattern != "" {
			include = append(include, regexp.MustCompile(pattern))
		}
	}
	discoverer := &impl{
		o:       o,
		exclude: exclude,
		include: include,
		client:  client,
		cache:   sync.Map{},
		logger:  log.With(logger, "discoverer", name),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: "nacos",
			Name:      "scrape_duration",
			Help:      "duration of service discovery process",
		}, []string{"service"}),
	}
	registerer.MustRegister(discoverer.duration)
	go discoverer.sync(context.Background())
	return discoverer, nil
}

type impl struct {
	o       *options
	exclude []*regexp.Regexp
	include []*regexp.Regexp

	cache    sync.Map
	mu       sync.Mutex
	client   naming_client.INamingClient
	logger   log.Logger
	duration *prometheus.HistogramVec
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
	now := time.Now()
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
	impl.duration.WithLabelValues("all").Observe(float64(time.Since(now).Seconds()))
	return ret, nil
}

func (impl *impl) getTargetgroupForService(s string) ([]*targetgroup.Group, error) {
	service, err := impl.client.GetService(vo.GetServiceParam{ServiceName: s})
	if err != nil {
		return nil, err
	}
	return nacos.Transform(service)
}

func (impl *impl) sync(ctx context.Context) error {
	ticker := time.NewTicker(impl.o.interval)
	defer ticker.Stop()

	q := make(chan struct{}, 1)
	q <- struct{}{}

	for {
		select {
		case <-ticker.C:
			q <- struct{}{}
		case <-q:
			if err := func() error {
				level.Debug(impl.logger).Log("msg", "refreshing targetgroups in cache")
				impl.mu.Lock()
				defer impl.mu.Unlock()
				services, err := impl.listServices(ctx)
				if err != nil {
					return err
				}
				eg, _ := errgroup.WithContext(ctx)
				for i := range services {
					s := services[i]
					eg.Go(func() error {
						now := time.Now()
						tgs, err := impl.getTargetgroupForService(s)
						if err != nil {
							return err
						}
						impl.duration.WithLabelValues(s).Observe(float64(time.Since(now).Seconds()))
						impl.cache.Store(s, tgs)
						return nil
					})
				}
				return eg.Wait()
			}(); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (impl *impl) Refresh(ctx context.Context, q url.Values) ([]*targetgroup.Group, error) {
	tgs, err := impl.refresh(ctx, q)
	if err != nil {
		return nil, err
	}
	return utils.Grouping(tgs), nil
}

func (impl *impl) refresh(_ context.Context, q url.Values) ([]*targetgroup.Group, error) {
	if sn := q.Get("serviceName"); sn != "" {
		if v, ok := impl.cache.Load(sn); ok {
			return v.([]*targetgroup.Group), nil
		}
		tgs, err := impl.getTargetgroupForService(sn)
		if err != nil {
			return nil, err
		}
		impl.cache.Store(sn, tgs)
		return tgs, nil
	}
	impl.mu.Lock()
	defer impl.mu.Unlock()

	var tgs []*targetgroup.Group
	impl.cache.Range(func(key, value any) bool {
		tgs = append(tgs, value.([]*targetgroup.Group)...)
		return true
	})
	return tgs, nil
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
