# mgmt

<!--
Mgmt
Copyright (C) 2013-2016+ James Shubin and the project contributors
Written by James Shubin <james@shubin.ca> and the project contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

## Prometheus support

Mgmt comes with a built-in prometheus support. It is disabled by default, and
can be enabled with the `--prometheus` command line switch.

By default, the prometheus instance will listen on [`127.0.0.1:9233`][pd]. You
can change this setting by using the `--prometheus-listen` cli option:

To have mgmt prometheus bind interface on 0.0.0.0:45001, use:
`./mgmt r --prometheus --prometheus-listen :45001`

## Metrics

Mgmt exposes three kinds of resources: _go_ metrics, _etcd_ metrics and _mgmt_
metrics.

### go metrics

We use the [prometheus go_collector][pgc] to expose go metrics. Those metrics
are mainly useful for debugging and perf testing.

### etcd metrics

mgmt exposes etcd metrics. Read more in the [upstream documentation][etcdm]

### mgmt metrics

Here is a list of the metrics we provide:

- `mgmt_resources_total`: The number of resources that mgmt is managing
- `mgmt_checkapply_total`: The number of CheckApply's that mgmt has run
- `mgmt_failures_total`: The number of resources that have failed
- `mgmt_failures_current`: The number of resources that have failed
- `mgmt_process_start_time_seconds`: Start time of the process since unix epoch in seconds

For each metric, you will get some extra labels:

- `kind`: The kind of mgmt resource

For `mgmt_checkapply_total`, those extra labels are set:

- `eventful`: "true" or "false", if the CheckApply triggered some changes
- `errorful`: "true" or "false", if the CheckApply reported an error
- `apply`: "true" or "false", if the CheckApply ran in apply or noop mode

## Alerting

You can use prometheus to alert you upon changes or failures. We do not provide
such templates yet, but we plan to provide some examples in this repository.
Patches welcome!

## Grafana

We do not have grafana dashboards yet. Patches welcome!

## External resources

- [prometheus website](https://prometheus.io/)
- [prometheus documentation](https://prometheus.io/docs/introduction/overview/)
- [prometheus best practices regarding metrics
  naming](https://prometheus.io/docs/practices/naming/)
- [grafana website](http://grafana.org/)

[pgc]: https://github.com/prometheus/client_golang/blob/master/prometheus/go_collector.go
[etcdm]: https://coreos.com/etcd/docs/latest/metrics.html
[pd]: https://github.com/prometheus/prometheus/wiki/Default-port-allocation
