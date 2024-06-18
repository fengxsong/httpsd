# HTTPSD

HTTP service discovery adapter for prometheus. This project was initially created because nacos does not offer full prometheus support at the [latest version](https://github.com/alibaba/nacos/releases/tag/2.3.2).

## Implement your own transformer. :)

```golang
type Transformer interface {
	Name() string
	TargetURL(string, url.Values) string
	HTTPMethod() string
	Transform([]byte) ([]*targetgroup.Group, error)
}
```

## [http client config](https://github.com/prometheus/common/blob/main/config)

## integrate with prometheus, example

```yaml
scrape_configs:
  - job_name: freeswitch
    scrape_interval: 15s
    http_sd_configs:
      - url: 'http://localhost:8080/sd?transformer=nacos&serviceName=fsproxy&namespaceId=test&pretty=true'
    metrics_path: /probe
    relabel_configs:
      - source_labels: ['__meta_ip', '__meta_port']
        separator: ':'
        target_label: __param_target
      - source_labels: ['__param_target']
        regex: '(.+)'
        replacement: 'tcp://${1}'
        target_label: __param_target
      - source_labels: ['__meta_password']
        target_label: __param_password
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: localhost:9282
```

## roadmap

no specific roadmap :)

## contributions

🐛 fix and feature request are welcome!
