Kubernetes manifests for bulk-service

Files:
- deployment-dev.yaml — development deployment (1 replica, small resources)
- deployment-staging.yaml — staging deployment (2 replicas)
- deployment-prod.yaml — production deployment (3 replicas, larger resources)
- service.yaml — ClusterIP service exposing port 8080
- hpa-prod.yaml — example HorizontalPodAutoscaler for production
- pdb-prod.yaml — PodDisruptionBudget for production

Notes:
- Secrets (DATABASE_URL, JWT_SECRET, etc.) must be created in `bulk-service-secrets` secret in the target namespace.
- Adjust `image` fields to point to your registry and set `imagePullSecrets` if necessary.
- These manifests expect the application to expose `/health` and `/ready` endpoints. If `/ready` is not implemented, see repository file `internal/transport/http/router.go`.

Apply for dev:
  kubectl apply -f deploy/k8s/service.yaml -f deploy/k8s/deployment-dev.yaml

Apply for prod (example):
  kubectl apply -f deploy/k8s/service.yaml -f deploy/k8s/deployment-prod.yaml -f deploy/k8s/hpa-prod.yaml -f deploy/k8s/pdb-prod.yaml

