Alertmanager provisioning (example)

This folder contains an example `alertmanager-config.yaml` with a Slack receiver.

Important:
- Replace the example `api_url` with a real Slack webhook or provide it via a Kubernetes Secret and mount it into the Alertmanager Pod.

Mounting example (k8s):

1) Create secret (replace webhook):

```powershell
kubectl apply -f deploy/alertmanager/secret-slack.yaml
```

2) Mount secret as file into Alertmanager pod (example snippet for Deployment/StatefulSet):

```yaml
volumeMounts:
  - name: alertmanager-secrets
	mountPath: /etc/alertmanager/secrets
	readOnly: true
volumes:
  - name: alertmanager-secrets
	secret:
	  secretName: alertmanager-slack
```

3) Update `alertmanager-config.yaml` or your deployment tooling to ensure the Slack webhook is read from `/etc/alertmanager/secrets/slack_api_url` and injected into the final Alertmanager config before starting Alertmanager. Many operators accept a Secret or ConfigMap; consult your deployment method.

Apply example:

```powershell
# create secret for slack webhook
kubectl create secret generic alertmanager-slack --from-literal=slack_api_url='https://hooks.slack.com/services/…' -n monitoring

# apply config (if using kube-prometheus/operator, set the config via CRD or Secret provider)
kubectl apply -f deploy/alertmanager/alertmanager-config.yaml -n monitoring
```

Tips:
- When running Alertmanager under k8s, store sensitive values as Secrets and reference them in the Alertmanager manifest.
- Test alerts by sending a test alert to Alertmanager API: `curl -XPOST -d @alert.json http://alertmanager:9093/api/v1/alerts`.

