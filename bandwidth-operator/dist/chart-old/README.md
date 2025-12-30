# Vaclab Bandwidth Operator Helm Chart

A Helm chart for deploying the Vaclab Bandwidth Operator - a Kubernetes operator that creates custom bandwidth resources for each node in the cluster.

## Prerequisites

- Kubernetes v1.11.3+
- Helm 3.0+
- kubectl configured with cluster access

## Installation

### Add the Helm Chart

```bash
# Install from local chart
helm install vaclab-bandwidth-operator ./dist/chart -n vaclab-system --create-namespace
```

### Install with custom values

```bash
helm install vaclab-bandwidth-operator ./dist/chart \
  -n vaclab-system --create-namespace \
  --set manager.image.tag=v0.1.0 \
  --set manager.bandwidth.network.uplink=10000
```

## Configuration

### Bandwidth Settings

Configure default bandwidth limits for nodes:

```yaml
manager:
  bandwidth:
    # Local bandwidth (OVS/bridge) in Mbps
    local:
      uplink: 20000    # 20 Gbps
      downlink: 20000
    # Network bandwidth (physical NIC) in Mbps
    network:
      uplink: 1000     # 1 Gbps
      downlink: 1000
```

### CNI-Specific Annotations

#### For KubeOVN (default)

```yaml
manager:
  annotations:
    egress: "ovn.kubernetes.io/egress_rate"
    ingress: "ovn.kubernetes.io/ingress_rate"
```

#### For Calico, Cilium, or standard Kubernetes CNI

```yaml
manager:
  annotations:
    egress: "kubernetes.io/egress-bandwidth"
    ingress: "kubernetes.io/ingress-bandwidth"
```

### Image Configuration

```yaml
manager:
  image:
    repository: katakuri100/vaclab-bandwidth-operator
    tag: latest
    pullPolicy: IfNotPresent
```

### Leader Election (for HA)

```yaml
manager:
  replicas: 3
  leaderElection:
    enabled: true
```

### Resources

```yaml
manager:
  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `manager.replicas` | int | `1` | Number of replicas |
| `manager.image.repository` | string | `"katakuri100/vaclab-bandwidth-operator"` | Image repository |
| `manager.image.tag` | string | `"latest"` | Image tag |
| `manager.image.pullPolicy` | string | `"IfNotPresent"` | Image pull policy |
| `manager.bandwidth.local.uplink` | int | `20000` | Default local uplink bandwidth (Mbps) |
| `manager.bandwidth.local.downlink` | int | `20000` | Default local downlink bandwidth (Mbps) |
| `manager.bandwidth.network.uplink` | int | `1000` | Default network uplink bandwidth (Mbps) |
| `manager.bandwidth.network.downlink` | int | `1000` | Default network downlink bandwidth (Mbps) |
| `manager.annotations.egress` | string | `"ovn.kubernetes.io/egress_rate"` | Pod egress bandwidth annotation |
| `manager.annotations.ingress` | string | `"ovn.kubernetes.io/ingress_rate"` | Pod ingress bandwidth annotation |
| `manager.leaderElection.enabled` | bool | `false` | Enable leader election |
| `crd.enable` | bool | `true` | Install CRDs with the chart |
| `crd.keep` | bool | `true` | Keep CRDs on uninstall |
| `metrics.enable` | bool | `true` | Enable metrics endpoint |
| `metrics.port` | int | `8443` | Metrics server port |
| `rbacHelpers.enable` | bool | `false` | Install helper RBAC roles |

## Usage

### Check operator status

```bash
kubectl get pods -n vaclab-system
kubectl logs -n vaclab-system -l control-plane=controller-manager
```

### View Bandwidth resources

```bash
kubectl get bandwidths
kubectl describe bandwidth <node-name>
```

### Example Pod with bandwidth annotation (KubeOVN)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  annotations:
    ovn.kubernetes.io/ingress_rate: "100"   # 100 Mbps
    ovn.kubernetes.io/egress_rate: "100"    # 100 Mbps
spec:
  containers:
  - name: app
    image: nginx
```

## Uninstall

```bash
# Remove the operator (keeps CRDs by default)
helm uninstall vaclab-bandwidth-operator -n vaclab-system

# Remove CRDs manually if needed
kubectl delete crd bandwidths.networking.vaclab.org
```

## Upgrade

```bash
# Regenerate chart after code changes
cd bandwidth-operator
kubebuilder edit --plugins=helm/v2-alpha --force

# Upgrade the release
helm upgrade vaclab-bandwidth-operator ./dist/chart -n vaclab-system
```

## Troubleshooting

### Check operator logs
```bash
kubectl logs -n vaclab-system -l control-plane=controller-manager -f
```

### Verify CRD is installed
```bash
kubectl get crd bandwidths.networking.vaclab.org
```

### Check RBAC permissions
```bash
kubectl auth can-i get bandwidths --as=system:serviceaccount:vaclab-system:vaclab-bandwidth-operator-controller-manager
```

## License

Copyright 2025. Licensed under the Apache License, Version 2.0.
