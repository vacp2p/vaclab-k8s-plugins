# 🚀 Vaclab Kubernetes Plugins

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.11.3+-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)
[![Go](https://img.shields.io/badge/Go-1.24.6+-00ADD8?logo=go&logoColor=white)](https://golang.org/)

> Custom Kubernetes operators and scheduler plugins for bandwidth-aware workload orchestration in the Vaclab environment.

## Overview

This repository provides Kubernetes extensions designed to enable **deterministic bandwidth management** and **intelligent scheduling** in research and laboratory environments where network reproducibility is critical. 

### Motivation

In the Vaclab environment, traditional Kubernetes scheduling cannot guarantee:
- **Bandwidth accountability**: tracking actual network resource usage per node
- **Strict bandwidth enforcement**: preventing oversubscription of network links
- **Scheduling-aware bandwidth allocation**: rejecting pod placement when bandwidth is unavailable
- **Pod locality optimization**: co-locating related pods (StatefulSets, Deployments) on the same physical node to leverage local virtual bridges

Standard Kubernetes networking lacks strict bandwidth reservation mechanisms, making experiments **non-reproducible** due to network fluctuations. Our goal here is to propose a clean workaround to this problem.

---

## Architecture

### Components

1. **[Bandwidth Operator](#-bandwidth-operator)** ([`bandwidth-operator/`](./bandwidth-operator/))
   - Custom Resource Definition (CRD) and controller
   - Per-node bandwidth tracking and enforcement
   - Pod lifecycle monitoring and bandwidth accounting

2. **[Bandwidth-Aware Scheduler](#-bandwidth-aware-scheduler-plugin)** (WIP)
   - Custom scheduler plugins for the Kubernetes scheduler
   - Node filtering based on available bandwidth
   - Locality-aware scheduling for grouped workloads

---

## 🔧 Bandwidth Operator

**Location:** [`bandwidth-operator/`](./bandwidth-operator/)

The Bandwidth Operator introduces a new Kubernetes Custom Resource called `Bandwidth` that represents the network capacity of each cluster node. It monitors pod lifecycle events and maintains real-time bandwidth allocation state.

### Key Features

- **Dual Bandwidth Tracking:**
  - **Local Bandwidth** — Virtual switch capacity (e.g., OVS) for intra-node traffic
  - **Network Bandwidth** — Physical NIC capacity (egress/ingress) for inter-node traffic

- **Automatic Resource Management:**
  - Creates `Bandwidth` CRs for each node automatically
  - Watches pod creation/deletion events
  - Parses bandwidth annotations from pod specifications
  - Updates bandwidth allocation in real-time
  - Maintains reservation lists for active pods

- **Bandwidth Accounting:**
  - Tracks used bandwidth per pod
  - Computes available bandwidth dynamically
  - Prevents scheduling when capacity is exhausted

### Installation

#### Prerequisites

- Kubernetes cluster v1.11.3+
- kubectl configured with cluster access
- Docker v17.03+
- Go v1.24.6+ (for building from source)

#### Quick Install

Use the pre-built installer manifest:

```bash
kubectl apply -f install.bw.operator.yaml
```

#### Build from Source

1. **Navigate to the operator directory:**
   ```bash
   cd bandwidth-operator
   ```

2. **Build and push the Docker image:**
   ```bash
   make docker-build docker-push IMG=<your-registry>/vaclab-bandwidth-operator:latest
   ```
   
   Replace `<your-registry>` with your container registry (e.g., `katakuri100`, `docker.io/myuser`, `gcr.io/myproject`).

3. **Generate the installer manifest:**
   ```bash
   make build-installer IMG=<your-registry>/vaclab-bandwidth-operator:latest
   ```
   
   This creates an `install.yaml` file in `bandwidth-operator/dist/`.

4. **Deploy to your cluster:**
   ```bash
   kubectl apply -f dist/install.yaml
   ```

### 🔍 Custom Resource Definition

The `Bandwidth` CRD defines node network capacity and current allocations:

```yaml
apiVersion: networking.vaclab.org/v1
kind: Bandwidth
metadata:
  name: node-name
spec:
  capacity:
    local:
      ulMbps: 20000  # OVS uplink capacity in Mbps
      dlMbps: 20000  # OVS downlink capacity in Mbps
    network:
      ulMbps: 1000   # Physical NIC egress capacity
      dlMbps: 1000   # Physical NIC ingress capacity
  reservations: []   # List of pod bandwidth reservations
```

**Example:** See [`bandwidth-operator/config/samples/networking_v1_bandwidth.yaml`](./bandwidth-operator/config/samples/networking_v1_bandwidth.yaml)

### 🏷️ Pod Annotations

Pods request bandwidth using annotations (this will be enforced later at veth level by the CNI in use):

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nimlibp2p-xyz
  annotations:
    kubernetes.io/egress-bandwidth: "100M"     # Mbps
    kubernetes.io/egress-bandwidth: "100M"   # Mbps
spec:
  # ... pod spec
```

## Bandwidth-Aware Scheduler Plugin (WIP)

**Status:** 🚧 Under Development

The scheduler plugin extends the default Kubernetes scheduler to make bandwidth-aware scheduling decisions.

```
Scheduler Plugin → Filters Nodes by Bandwidth
                ↓
         Selects Optimal Node
                ↓
    Updates Bandwidth CR (reservation)
                ↓
  Bandwidth Operator Reconciles
                ↓
    Updates Used/Available Bandwidth
                ↓
         Adds Pod to Reservation List
```

## Resources

- **Kubernetes Custom Resources:** [Official Documentation](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
- **Kubebuilder:** [Book](https://book.kubebuilder.io/)
- **Kubernetes Scheduler Framework:** [Documentation](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/)


