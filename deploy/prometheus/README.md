Prometheus provisioning for Bulk Service

Files in this folder:
- `configmap-bulk-rules.yaml` - ConfigMap that contains recording and alert rules for Bulk Service.
- `bulk-service-prometheusrule.yaml` - PrometheusRule CRD for prometheus-operator / kube-prometheus stacks.

How to apply
1. Ensure Prometheus is deployed in `monitoring` namespace (or change namespace in the ConfigMap manifest).
2. Apply the ConfigMap:

```powershell
kubectl apply -f deploy/prometheus/configmap-bulk-rules.yaml
```

If you are using the prometheus-operator (kube-prometheus stack) you can apply the provided PrometheusRule CRD instead:

```powershell
kubectl apply -f deploy/prometheus/bulk-service-prometheusrule.yaml -n monitoring
```

3. Mount the ConfigMap into your Prometheus server Pod (example for kube-prometheus / prometheus-operator: create a PrometheusRule CRD or update additionalScrapeConfigs). If using a vanilla Prometheus deployment, mount the ConfigMap to `/etc/prometheus/rules` and restart Prometheus.

Example (kubectl create configmap approach):
```
kubectl create configmap bulk-service-prom-rules --from-file=recording_rules.yaml=deploy/prometheus/rules/bulk_service_recording_rules.yaml --from-file=alert_rules.yaml=deploy/prometheus/rules/bulk_service_alerts.yaml -n monitoring
```

Notes
- The ConfigMap in this repo contains rules under `recording_rules.yaml` and `alert_rules.yaml` keys. Adjust Prometheus configuration to load both files from the mounted path.
- After applying, check Prometheus UI -> Status -> Rules to ensure rules are loaded.

