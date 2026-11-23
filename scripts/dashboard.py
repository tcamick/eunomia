"""Writes k8s/monitoring/eunomia-dashboard.json. Run from the repository root: python3 -I scripts/dashboard.py"""
import json

DS = {"type": "prometheus", "uid": "${datasource}"}
panels = []
y = 0


def add(title, kind, w, h, exprs, x=0, unit=None, extra=None, desc=None):
    global y
    p = {
        "id": len(panels) + 1,
        "title": title,
        "type": kind,
        "datasource": DS,
        "gridPos": {"x": x, "y": y, "w": w, "h": h},
        "targets": [
            {"datasource": DS, "refId": chr(65 + i), "expr": e, "legendFormat": lf}
            for i, (e, lf) in enumerate(exprs)
        ],
        "fieldConfig": {"defaults": {}, "overrides": []},
        "options": {},
    }
    if desc:
        p["description"] = desc
    if unit:
        p["fieldConfig"]["defaults"]["unit"] = unit
    if extra:
        for k, v in extra.items():
            p[k] = v
    panels.append(p)
    return p


def row(title):
    global y
    panels.append({"id": len(panels) + 1, "type": "row", "title": title, "collapsed": False,
                   "gridPos": {"x": 0, "y": y, "w": 24, "h": 1}, "panels": []})
    y += 1


okmap = {"fieldConfig": {"defaults": {
    "thresholds": {"mode": "absolute", "steps": [{"color": "red", "value": None}, {"color": "green", "value": 1}]},
    "mappings": [{"type": "value", "options": {"0": {"text": "failed"}, "1": {"text": "ok"}}}]}, "overrides": []}}

row("Jobs (from the job history in SQLite, read by the server at each scrape)")
add("Last run", "stat", 12, 6, [("eunomia_job_last_run_ok", "{{job}}")], extra=okmap,
    desc="Whether each job's last run succeeded.")
add("Hours since last success", "stat", 12, 6,
    [("(time() - eunomia_job_last_success_timestamp_seconds) / 3600", "{{job}}")], x=12, unit="h",
    desc="A job that stops succeeding shows here first.")
y += 6
add("History readable", "stat", 6, 4, [("eunomia_job_history_up", "")], extra=okmap,
    desc="0 means the server could not read the job history at the last scrape.")
add("Server up", "stat", 6, 4, [('up{namespace="eunomia"}', "{{pod}}")], x=6, extra=okmap)
add("Last push age", "stat", 12, 4,
    [('time() - push_time_seconds{job="eunomia-batch"}', "{{batch}}")], x=12, unit="s",
    desc="How long ago each batch last pushed to the Pushgateway (beholder ADR 0002).")
y += 4
row("Outside sources (pushed by each batch run at its end; values are that run's, not a running total)")
add("Requests in each batch's last run", "table", 24, 8,
    [('eunomia_fetch_requests_total{job="eunomia-batch"}', "")],
    extra={"transformations": [{"id": "labelsToFields", "options": {}}]},
    desc="Per source and result. A run of server_error or transport_error means a source is down or has moved.")
y += 8
row("App")
add("Requests by route and code", "timeseries", 24, 8,
    [("sum by (route, code) (rate(eunomia_http_requests_total[5m]))", "{{route}} {{code}}")], unit="reqps")
y += 8

dash = {
    "uid": "eunomia-overview",
    "title": "Eunomia",
    "tags": ["eunomia"],
    "timezone": "browser",
    "editable": True,
    "schemaVersion": 39,
    "refresh": "1m",
    "time": {"from": "now-24h", "to": "now"},
    "graphTooltip": 1,
    "templating": {"list": [{"name": "datasource", "label": "Data source", "type": "datasource",
                             "query": "prometheus", "current": {}, "hide": 0}]},
    "annotations": {"list": []},
    "panels": panels,
}
with open("k8s/monitoring/eunomia-dashboard.json", "w") as f:
    json.dump(dash, f, indent=1)
    f.write("\n")
