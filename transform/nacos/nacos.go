package nacos

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/fengxsong/httpsd/transform"
)

const name = "nacos"

type impl struct{}

func (impl) Name() string { return name }

func (impl) TargetURL(base string, q url.Values) string {
	return fmt.Sprintf("%s/nacos/v1/ns/instance/list?%s", base, q.Encode())
}

func (impl) HTTPBody() io.Reader { return nil }

func (impl) HTTPMethod() string { return http.MethodGet }

func (impl) Transform(b []byte) ([]*targetgroup.Group, error) {
	var instances Service
	if err := json.Unmarshal(b, &instances); err != nil {
		return nil, err
	}
	var targetGroups []*targetgroup.Group
	for _, instance := range instances.Hosts {
		g := &targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue(net.JoinHostPort(instance.Ip, strconv.Itoa(int(instance.Port))))},
			},
			Labels: model.LabelSet{
				labelName("cluster"): model.LabelValue(instance.ClusterName),
				labelName("service"): model.LabelValue(instance.ServiceName),
				labelName("group"):   model.LabelValue(instances.GroupName),
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
	return model.LabelName(fmt.Sprintf("%s%s%s_%s", model.MetaLabelPrefix, name, placeholder, transform.FormalizeLabelName(k)))
}

func init() {
	if err := transform.Register(impl{}); err != nil {
		panic(err)
	}
}
