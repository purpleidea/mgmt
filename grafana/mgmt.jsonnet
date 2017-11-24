local grafana = import "grafonnet/grafana.libsonnet";
local dashboard = grafana.dashboard;
local template = grafana.template;
local row = grafana.row;
local text = grafana.text;
local graphPanel = grafana.graphPanel;
local singlestat = grafana.singlestat;
local prometheus = grafana.prometheus;

dashboard.new(
    "Mgmt Overview",
    tags=["Config Management", "mgmt"],
    refresh="10s",
    time_from="now-10m",
)
+ dashboard.addTemplate(
    template.datasource(
        "PROMETHEUS_DS",
        "prometheus",
        "Prometheus",
        hide="label",
    )

)
+ dashboard.addRow(
    row.new(height="245px")
    + row.addPanel(
        text.new(
            content=std.extVar("logo"),
            transparent=true,
            mode="html",
            id=3,
            span=2,
        )
    )
    + row.addPanel(
        singlestat.new(
            "graph age",
            id=1,
            datasource="$PROMETHEUS_DS",
            valueName="current",
            format="s",
            span=2,
        )
        + singlestat.addTarget(
            prometheus.target(
                "time()-mgmt_graph_start_time_seconds",
            )
        )
    )
    + row.addPanel(
        singlestat.new(
            "resources",
            id=2,
            datasource="$PROMETHEUS_DS",
            valueName="current",
            span=2,
        )
        + singlestat.addTarget(
            prometheus.target(
                "sum(mgmt_resources)",
            )
        )
    )
    + row.addPanel(
        singlestat.new(
            "recent changes",
            id=4,
            datasource="$PROMETHEUS_DS",
            valueName="current",
            span=2,
        )
        + singlestat.addTarget(
            prometheus.target(
                "sum(delta(mgmt_checkapply_total{eventful=\"true\"}[5m]))",
            )
        )
    )
    + row.addPanel(
        singlestat.new(
            "hard failures",
            id=44,
            datasource="$PROMETHEUS_DS",
            valueName="current",
            span=2,
        )
        + singlestat.addTarget(
            prometheus.target(
                "sum(mgmt_failures_total{failure=\"hard\"})",
            )
        )
    )
)
+ dashboard.addRow(
    row.new()
    + row.addPanel(
        graphPanel.new(
            "Resources",
            id=6,
            span=6,
            format="short",
            fill=6,
            min=0,
            decimals=0,
            datasource="-- Mixed --",
            legend_values=true,
            legend_min=true,
            legend_max=true,
            legend_current=true,
            legend_total=false,
            legend_avg=true,
            legend_alignAsTable=true,
            legend_hideZero=true,
            legend_hideEmpty=true,
            stack=true,
        )
        + graphPanel.addTarget(
            prometheus.target(
                "mgmt_resources",
                datasource="$PROMETHEUS_DS",
                legendFormat="{{kind}}"
            )
        )
    )
    + row.addPanel(
        graphPanel.new(
            "CheckApplies",
            id=10,
            span=6,
            format="short",
            fill=6,
            min=0,
            decimals=0,
            datasource="-- Mixed --",
            legend_values=true,
            legend_min=true,
            legend_max=true,
            legend_current=true,
            legend_total=false,
            legend_avg=true,
            legend_alignAsTable=true,
            legend_hideZero=true,
            legend_hideEmpty=true,
            stack=true,
        )
        + graphPanel.addTarget(
            prometheus.target(
                "sum(delta(mgmt_checkapply_total{eventful=\"true\"}[1m])) by (kind)",
                datasource="$PROMETHEUS_DS",
                legendFormat="{{kind}} (eventful)",
            )
        )
        + graphPanel.addTarget(
            prometheus.target(
                "sum(delta(mgmt_checkapply_total{eventful=\"true\"}[1m])) by (kind)",
                datasource="$PROMETHEUS_DS",
                legendFormat="{{kind}} (eventless)",
            )
        )
    )
)
