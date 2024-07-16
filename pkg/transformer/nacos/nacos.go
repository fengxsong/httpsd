package nacos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	nacosmodel "github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/fengxsong/httpsd/pkg/transformer"
	"github.com/fengxsong/httpsd/pkg/utils"
)

const name = "nacos"

type impl struct{}

func (impl) Name() string { return name }

func (impl) SampleConfig() transformer.Config {
	return nil
}

func (impl) Init(_ transformer.Config) error { return nil }

func (impl) TargetURL(base string, q url.Values) (string, error) {
	serviceName := q.Get("serviceName")
	if serviceName == "" {
		return "", errors.New("serviceName is required")
	}
	qs := url.Values{}
	qs.Set("serviceName", serviceName)
	qs.Set("groupName", q.Get("groupName"))
	qs.Set("namespaceId", q.Get("namespaceId"))
	qs.Set("clusters", q.Get("clusters"))
	qs.Set("healthyOnly", q.Get("healthyOnly"))
	return fmt.Sprintf("%s/nacos/v1/ns/instance/list?%s", base, qs.Encode()), nil
}

func (impl) HTTPBody() io.Reader { return nil }

func (impl) HTTPMethod() string { return http.MethodGet }

func (impl) Transform(_ context.Context, b []byte) ([]*targetgroup.Group, error) {
	var instances nacosmodel.Service
	if err := json.Unmarshal(b, &instances); err != nil {
		return nil, err
	}
	return Transform(instances)
}

func Transform(service nacosmodel.Service) ([]*targetgroup.Group, error) {
	var targetGroups []*targetgroup.Group
	for _, instance := range service.Hosts {
		g := &targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue(net.JoinHostPort(instance.Ip, strconv.Itoa(int(instance.Port))))},
			},
			Labels: model.LabelSet{
				labelName("cluster"): model.LabelValue(instance.ClusterName),
				labelName("service"): model.LabelValue(instance.ServiceName),
				labelName("group"):   model.LabelValue(service.GroupName),
			},
		}
		for k, v := range instance.Metadata {
			g.Labels[labelName(k, "_metadata")] = model.LabelValue(v)
		}
		targetGroups = append(targetGroups, g)
	}
	return targetGroups, nil
}

func labelName(k string, args ...string) model.LabelName {
	placeholder := ""
	if len(args) > 0 {
		placeholder = args[0]
	}
	return model.LabelName(fmt.Sprintf("%s%s%s_%s", model.MetaLabelPrefix, name, placeholder, utils.FormalizeLabelName(k)))
}

func init() {
	if err := transformer.Register(&impl{}); err != nil {
		panic(err)
	}
}
