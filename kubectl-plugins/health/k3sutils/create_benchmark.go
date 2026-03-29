package k3sutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func CreateIperfHeadlessServices() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Headless service for hostnetwork iperf servers
	hostNetworkSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HOSTNETWORK_HEADLESS_SERVICE_NAME,
			Namespace: DEFAULT_NAMESPACE,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"app": "iperf-hostnetwork",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       int32(DEFAULT_IPERF_PORT),
					TargetPort: intstr.FromInt(DEFAULT_IPERF_PORT),
				},
			},
		},
	}

	// Headless service for podnetwork iperf servers
	podNetworkSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PODNETWORK_HEADLESS_SERVICE_NAME,
			Namespace: DEFAULT_NAMESPACE,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector: map[string]string{
				"app": "iperf-podnetwork",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       int32(DEFAULT_IPERF_PORT),
					TargetPort: intstr.FromInt(DEFAULT_IPERF_PORT),
				},
			},
		},
	}

	// Create hostnetwork headless service
	if _, err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Get(ctx, HOSTNETWORK_HEADLESS_SERVICE_NAME, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Create(ctx, hostNetworkSvc, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create headless service '%s': %v\n", HOSTNETWORK_HEADLESS_SERVICE_NAME, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] headless service '%s' created\n", HOSTNETWORK_HEADLESS_SERVICE_NAME)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get service '%s': %v\n", HOSTNETWORK_HEADLESS_SERVICE_NAME, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] headless service '%s' already exists\n", HOSTNETWORK_HEADLESS_SERVICE_NAME)
	}

	// Create podnetwork headless service
	if _, err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Get(ctx, PODNETWORK_HEADLESS_SERVICE_NAME, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Create(ctx, podNetworkSvc, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create headless service '%s': %v\n", PODNETWORK_HEADLESS_SERVICE_NAME, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] headless service '%s' created\n", PODNETWORK_HEADLESS_SERVICE_NAME)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get service '%s': %v\n", PODNETWORK_HEADLESS_SERVICE_NAME, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] headless service '%s' already exists\n", PODNETWORK_HEADLESS_SERVICE_NAME)
	}

	tw.Flush()
	return nil
}

func CreateIperfHostNetworkDaemonSet() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	hostNetworkLabels := map[string]string{
		"app":     "iperf-hostnetwork",
		"network": "host",
	}

	// DaemonSet for hostnetwork iperf servers
	hostNetworkDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_SERVERS_HN_DAEMONSET_NAME,
			Namespace: DEFAULT_NAMESPACE,
			Labels: map[string]string{
				"network": "host",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "iperf-hostnetwork",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: hostNetworkLabels,
				},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					Containers: []corev1.Container{
						{
							Name:  "iperf",
							Image: IPERF_SERVER_IMAGE,
							Args:  []string{"-s", "-p", fmt.Sprintf("%d", DEFAULT_IPERF_PORT)},
						},
					},
				},
			},
		},
	}

	dsClient := K3sClient.AppsV1().DaemonSets(DEFAULT_NAMESPACE)

	// Create hostnetwork DaemonSet
	if _, err := dsClient.Get(ctx, IPERF_SERVERS_HN_DAEMONSET_NAME, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := dsClient.Create(ctx, hostNetworkDS, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create daemonset '%s': %v\n", IPERF_SERVERS_HN_DAEMONSET_NAME, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] daemonset '%s' created\n", IPERF_SERVERS_HN_DAEMONSET_NAME)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get daemonset '%s': %v\n", IPERF_SERVERS_HN_DAEMONSET_NAME, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] daemonset '%s' already exists\n", IPERF_SERVERS_HN_DAEMONSET_NAME)
	}

	tw.Flush()
	return nil
}

func CreateIperfPodNetworkDaemonSet() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	podNetworkLabels := map[string]string{
		"app":     "iperf-podnetwork",
		"network": "pod",
	}

	// DaemonSet for podnetwork iperf servers
	podNetworkDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_SERVERS_PN_DAEMONSET_NAME,
			Namespace: DEFAULT_NAMESPACE,
			Labels: map[string]string{
				"network": "pod",
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "iperf-podnetwork",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podNetworkLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "iperf",
							Image: IPERF_SERVER_IMAGE,
							Args:  []string{"-s", "-p", fmt.Sprintf("%d", DEFAULT_IPERF_PORT)},
						},
					},
				},
			},
		},
	}

	dsClient := K3sClient.AppsV1().DaemonSets(DEFAULT_NAMESPACE)

	// Create podnetwork DaemonSet
	if _, err := dsClient.Get(ctx, IPERF_SERVERS_PN_DAEMONSET_NAME, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := dsClient.Create(ctx, podNetworkDS, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create daemonset '%s': %v\n", IPERF_SERVERS_PN_DAEMONSET_NAME, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] daemonset '%s' created\n", IPERF_SERVERS_PN_DAEMONSET_NAME)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get daemonset '%s': %v\n", IPERF_SERVERS_PN_DAEMONSET_NAME, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] daemonset '%s' already exists\n", IPERF_SERVERS_PN_DAEMONSET_NAME)
	}

	tw.Flush()
	return nil
}

func CreateIperfRolesAndBindings() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_BENCHMARK_SA,
			Namespace: DEFAULT_NAMESPACE,
		},
	}

	// Role for reading pods and nodes
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_NODE_POD_READER_ROLE,
			Namespace: DEFAULT_NAMESPACE,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	// RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_BENCHMARK_ROLE_BINDING,
			Namespace: DEFAULT_NAMESPACE,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      IPERF_BENCHMARK_SA,
				Namespace: DEFAULT_NAMESPACE,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     IPERF_NODE_POD_READER_ROLE,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	// Create ServiceAccount
	if _, err := K3sClient.CoreV1().ServiceAccounts(DEFAULT_NAMESPACE).Get(ctx, IPERF_BENCHMARK_SA, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.CoreV1().ServiceAccounts(DEFAULT_NAMESPACE).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create service account '%s': %v\n", IPERF_BENCHMARK_SA, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] service account '%s' created\n", IPERF_BENCHMARK_SA)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get service account '%s': %v\n", IPERF_BENCHMARK_SA, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] service account '%s' already exists\n", IPERF_BENCHMARK_SA)
	}

	// Create Role
	if _, err := K3sClient.RbacV1().Roles(DEFAULT_NAMESPACE).Get(ctx, IPERF_NODE_POD_READER_ROLE, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.RbacV1().Roles(DEFAULT_NAMESPACE).Create(ctx, role, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create role '%s': %v\n", IPERF_NODE_POD_READER_ROLE, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] role '%s' created\n", IPERF_NODE_POD_READER_ROLE)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get role '%s': %v\n", IPERF_NODE_POD_READER_ROLE, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] role '%s' already exists\n", IPERF_NODE_POD_READER_ROLE)
	}

	// Create RoleBinding
	if _, err := K3sClient.RbacV1().RoleBindings(DEFAULT_NAMESPACE).Get(ctx, IPERF_BENCHMARK_ROLE_BINDING, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.RbacV1().RoleBindings(DEFAULT_NAMESPACE).Create(ctx, roleBinding, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create role binding '%s': %v\n", IPERF_BENCHMARK_ROLE_BINDING, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] role binding '%s' created\n", IPERF_BENCHMARK_ROLE_BINDING)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get role binding '%s': %v\n", IPERF_BENCHMARK_ROLE_BINDING, err)
			tw.Flush()
			return err
		}
	} else {
		fmt.Fprintf(tw, "[INFO] role binding '%s' already exists\n", IPERF_BENCHMARK_ROLE_BINDING)
	}

	tw.Flush()
	return nil
}

func CreateIperfBencharkScriptConfigMap() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	benchmarkScript := `#!/bin/bash
set -e

# Defaults if ENV variables are not provided
PARALLEL=${IPERF_PARALLEL:-5}
DURATION=${IPERF_DURATION:-10}
INTRA_NODE=${INTRA_NODE_MODE:-false}
EXCLUDE_NODES=${EXCLUDE_NODES:-""}
DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Function to check if a node is in the exclude list
is_excluded() {
  local node=$1
  if [ -z "$EXCLUDE_NODES" ]; then
    return 1
  fi
  IFS=',' read -ra EXCLUDED <<< "$EXCLUDE_NODES"
  for excluded in "${EXCLUDED[@]}"; do
    if [ "$node" == "$excluded" ]; then
      return 0
    fi
  done
  return 1
}

# Initialize results array
RESULTS="[]"

echo "--- STARTING BENCHMARK RUN: $DATE ---" >&2
echo "Target Service: $TARGET_SVC" >&2
echo "My Node: $MY_NODE_NAME" >&2
echo "Config: Parallel=$PARALLEL, Duration=${DURATION}s, IntraNode=$INTRA_NODE" >&2
[ -n "$EXCLUDE_NODES" ] && echo "Excluded nodes: $EXCLUDE_NODES" >&2

run_benchmark() {
  local svc_label=$1; local port=$2; local mode=$3

  # Find targets
  echo ">>> Discovering pods for label: app=$svc_label" >&2
  TARGETS=$(kubectl get pods -l app=$svc_label -o json | jq -c '.items[] | {ip: .status.podIP, node: .spec.nodeName}')
  
  if [ -z "$TARGETS" ]; then
    echo "ERROR: No target pods found for label app=$svc_label" >&2
    exit 1
  fi

  echo "$TARGETS" | while read -r target; do
    DEST_IP=$(echo $target | jq -r .ip)
    DEST_NODE=$(echo $target | jq -r .node)

    # Check if destination node is in exclude list
    if is_excluded "$DEST_NODE"; then
      echo "Skipping excluded node ($DEST_NODE)" >&2
      continue
    fi

    # Intra-node mode: only test against the server on the SAME node
    if [ "$INTRA_NODE" == "true" ]; then
      if [ "$DEST_NODE" != "$MY_NODE_NAME" ]; then
        echo "Skipping remote node ($DEST_NODE) - intra-node mode" >&2
        continue
      fi
    else
      # Inter-node mode: skip self (same node)
      if [ "$DEST_NODE" == "$MY_NODE_NAME" ]; then 
        echo "Skipping self ($MY_NODE_NAME)" >&2
        continue
      fi
    fi

    for DIRECTION in "UL" "DL"; do
      for PROTO in "tcp" "udp"; do
        echo "----------------------------------------------------------" >&2
        echo "TESTING: $PROTO | $DIRECTION | $MY_NODE_NAME -> $DEST_NODE ($DEST_IP)" >&2
        
        # Directional Flag: -R is Reverse (Download for the client)
        DIR_FLAG=""
        [ "$DIRECTION" == "DL" ] && DIR_FLAG="-R"
        
        # Protocol specific flags
        EXTRA_FLAGS=""
        [ "$PROTO" == "udp" ] && EXTRA_FLAGS="-u -b 0"
        
        # Run iperf3: Using ENV-driven Duration and Parallelism
        RESULT=$(iperf3 -c $DEST_IP -p $port -t $DURATION -P $PARALLEL $DIR_FLAG $EXTRA_FLAGS -J 2>/dev/null || echo '{}')
        
        # Log summary to stderr
        echo "iperf3 summary for $PROTO $DIRECTION:" >&2
        echo "$RESULT" | jq .end.sum_received >&2

        # Extract metrics for JSON output
        if [ "$PROTO" == "tcp" ]; then
          THROUGHPUT=$(jq '.end.sum_received.bits_per_second // 0' <<< "$RESULT")
          RETRANSMITS=$(jq '.end.sum_sent.retransmits // 0' <<< "$RESULT")
          RTT_MIN=$(jq '(.end.streams[0].sender.min_rtt // .end.streams[0].receiver.min_rtt // 0) / 1000' <<< "$RESULT")
          RTT_AVG=$(jq '(.end.streams[0].sender.mean_rtt // .end.streams[0].receiver.mean_rtt // 0) / 1000' <<< "$RESULT")
          RTT_MAX=$(jq '(.end.streams[0].sender.max_rtt // .end.streams[0].receiver.max_rtt // 0) / 1000' <<< "$RESULT")
          AVG_CWND=$(jq '[.intervals[].streams[0].snd_cwnd | select(. != null)] | if length > 0 then add / length else 0 end' <<< "$RESULT")
          
          # Output test result as JSON line
          jq -c -n \
            --arg dest "$DEST_IP" \
            --arg dest_node "$DEST_NODE" \
            --arg proto "$PROTO" \
            --arg dir "$DIRECTION" \
            --argjson throughput "$THROUGHPUT" \
            --argjson retransmits "$RETRANSMITS" \
            --argjson avg_cwnd "$AVG_CWND" \
            --argjson rtt_min "$RTT_MIN" \
            --argjson rtt_avg "$RTT_AVG" \
            --argjson rtt_max "$RTT_MAX" \
            '{
              destination: $dest,
              dest_node: $dest_node,
              protocol: $proto,
              direction: $dir,
              throughput_bps: $throughput,
              retransmits: $retransmits,
              avg_cwnd_bytes: $avg_cwnd,
              rtt_min_ms: $rtt_min,
              rtt_avg_ms: $rtt_avg,
              rtt_max_ms: $rtt_max
            }'

          # Push to VictoriaMetrics
          CONGESTION=$(jq -r '.end.sender_tcp_congestion // .end.receiver_tcp_congestion // "unknown"' <<< "$RESULT")
          cat <<EOF | curl -s -X POST "$VM_FULL_PATH" --data-binary @- 2>&1 >/dev/null
iperf_throughput_bps{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",cc="$CONGESTION"} $THROUGHPUT
iperf_retransmits_total{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",cc="$CONGESTION"} $RETRANSMITS
iperf_rtt_ms{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",stat="min"} $RTT_MIN
iperf_rtt_ms{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",stat="avg"} $RTT_AVG
iperf_rtt_ms{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",stat="max"} $RTT_MAX
iperf_tcp_avg_cwnd_bytes{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="tcp",direction="$DIRECTION",cc="$CONGESTION"} $AVG_CWND
EOF
        else
          THROUGHPUT=$(jq '.end.sum_received.bits_per_second // 0' <<< "$RESULT")
          JITTER=$(jq '.end.sum_received.jitter_ms // 0' <<< "$RESULT")
          LOST_PCT=$(jq '.end.sum_received.lost_percent // 0' <<< "$RESULT")
          
          # Output test result as JSON line
          jq -c -n \
            --arg dest "$DEST_IP" \
            --arg dest_node "$DEST_NODE" \
            --arg proto "$PROTO" \
            --arg dir "$DIRECTION" \
            --argjson throughput "$THROUGHPUT" \
            --argjson jitter "$JITTER" \
            --argjson lost_pct "$LOST_PCT" \
            '{
              destination: $dest,
              dest_node: $dest_node,
              protocol: $proto,
              direction: $dir,
              throughput_bps: $throughput,
              jitter_ms: $jitter,
              packet_loss_pct: $lost_pct
            }'
          
          # Push to VictoriaMetrics
          cat <<EOF | curl -s -X POST "$VM_FULL_PATH" --data-binary @- 2>&1 >/dev/null
iperf_throughput_bps{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="udp",direction="$DIRECTION"} $THROUGHPUT
iperf_jitter_ms{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="udp",direction="$DIRECTION"} $JITTER
iperf_packet_loss_percent{src="$MY_NODE_NAME",dest="$DEST_NODE",mode="$mode",protocol="udp",direction="$DIRECTION"} $LOST_PCT
EOF
        fi

        # Push to VictoriaLogs
        LOG_PAYLOAD=$(jq -c \
          --arg src "$MY_NODE_NAME" \
          --arg dest "$DEST_NODE" \
          --arg mode "$mode" \
          --arg proto "$PROTO" \
          --arg dir "$DIRECTION" \
          '{
            "_time": "'$DATE'",
            "_msg": ("iperf3 " + $proto + " " + $dir + ": " + $src + " -> " + $dest),
            "app": "iperf-health",
            "src": $src,
            "dest": $dest,
            "protocol": $proto,
            "mode": $mode,
            "direction": $dir,
            "iperf_stats": .
          }' <<< "$RESULT")
        
        curl -s -L -X POST "$VL_FULL_PATH" \
          -H "Content-Type: application/json" \
          --data-binary "$LOG_PAYLOAD" >/dev/null 2>&1

        echo "Done with $PROTO $DIRECTION" >&2
      done
    done
  done
}

run_benchmark "$TARGET_SVC" "$TARGET_PORT" "$BENCHMARK_MODE"
echo "--- ALL JOBS FINISHED ---" >&2
`

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_BENCHMARK_CONFIG_MAP,
			Namespace: DEFAULT_NAMESPACE,
		},
		Data: map[string]string{
			"benchmark.sh": benchmarkScript,
		},
	}

	if _, err := K3sClient.CoreV1().ConfigMaps(DEFAULT_NAMESPACE).Get(ctx, IPERF_BENCHMARK_CONFIG_MAP, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			if _, err := K3sClient.CoreV1().ConfigMaps(DEFAULT_NAMESPACE).Create(ctx, configMap, metav1.CreateOptions{}); err != nil {
				fmt.Fprintf(tw, "[ERROR] failed to create configmap '%s': %v\n", IPERF_BENCHMARK_CONFIG_MAP, err)
				tw.Flush()
				return err
			}
			fmt.Fprintf(tw, "[INFO] configmap '%s' created\n", IPERF_BENCHMARK_CONFIG_MAP)
		} else {
			fmt.Fprintf(tw, "[ERROR] failed to get configmap '%s': %v\n", IPERF_BENCHMARK_CONFIG_MAP, err)
			tw.Flush()
			return err
		}
	} else {
		// Update existing ConfigMap
		if _, err := K3sClient.CoreV1().ConfigMaps(DEFAULT_NAMESPACE).Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
			fmt.Fprintf(tw, "[ERROR] failed to update configmap '%s': %v\n", IPERF_BENCHMARK_CONFIG_MAP, err)
			tw.Flush()
			return err
		}
		fmt.Fprintf(tw, "[INFO] configmap '%s' updated\n", IPERF_BENCHMARK_CONFIG_MAP)
	}

	tw.Flush()
	return nil
}

// WaitForPodCompletion waits for a pod to complete (Succeeded or Failed) and returns the logs
func WaitForPodCompletion(podName string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for pod '%s' to complete", podName)
		default:
			pod, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to get pod '%s': %v", podName, err)
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded, corev1.PodFailed:
				// Pod completed, get logs
				logs, err := GetPodLogs(podName)
				if err != nil {
					return "", fmt.Errorf("failed to get logs for pod '%s': %v", podName, err)
				}
				return logs, nil
			case corev1.PodPending, corev1.PodRunning:
				// Still running, wait
				time.Sleep(5 * time.Second)
			default:
				return "", fmt.Errorf("pod '%s' is in unexpected phase: %s", podName, pod.Status.Phase)
			}
		}
	}
}

// GetPodLogs retrieves logs from a pod
func GetPodLogs(podName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).GetLogs(podName, &corev1.PodLogOptions{})
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get log stream: %v", err)
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		return "", fmt.Errorf("failed to read logs: %v", err)
	}

	return buf.String(), nil
}

func ParseBenchmarkLogs(logs string, config BenchmarkConfig, benchmarkMode string, sourceNode string) (*BenchmarkReport, error) {
	report := &BenchmarkReport{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		SourceNode:    sourceNode,
		BenchmarkMode: benchmarkMode,
		Parallel:      config.NumberOfConnections,
		Duration:      config.DurationSeconds,
		Results:       []IperfTestResult{},
	}

	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		var result IperfTestResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			// Skip lines that aren't valid JSON results
			continue
		}

		// Only add if it has valid data
		if result.Destination != "" {
			report.Results = append(report.Results, result)
		}
	}

	return report, nil
}

// CreateBenchmarkResultConfigMap creates a ConfigMap with benchmark results
func CreateBenchmarkResultConfigMap(report *BenchmarkReport, suffix string) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Serialize report to JSON
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %v", err)
	}

	configMapName := CONFIG_MAP_PREFIX + suffix + "-" + time.Now().Format("20060102-150405")

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: DEFAULT_NAMESPACE,
			Labels: map[string]string{
				"app":            "iperf-benchmark",
				"benchmark-mode": report.BenchmarkMode,
				"source-node":    report.SourceNode,
			},
		},
		Data: map[string]string{
			"report.json": string(reportJSON),
		},
	}

	if _, err := K3sClient.CoreV1().ConfigMaps(DEFAULT_NAMESPACE).Create(ctx, configMap, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create result configmap '%s': %v\n", configMapName, err)
		tw.Flush()
		return err
	}

	fmt.Fprintf(tw, "[INFO] benchmark results saved to configmap '%s'\n", configMapName)
	tw.Flush()
	return nil
}

func GetSourceNodeFromPod(podName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pod, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod '%s': %v", podName, err)
	}

	return pod.Spec.NodeName, nil
}

// CreateIperfHostNetworkBenchmarkPod creates a Pod for running iperf benchmark over host network
func CreateIperfHostNetworkBenchmarkPod(config BenchmarkConfig) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	defaultMode := int32(0755)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_CLIENT_HOSTNETWORK_NAME,
			Namespace: DEFAULT_NAMESPACE,
			Labels: map[string]string{
				"app":  "iperf-benchmark",
				"mode": "host-network",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
						{
							Weight: 100,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-role.kubernetes.io/control-plane",
										Operator: corev1.NodeSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			HostNetwork:        true,
			DNSPolicy:          corev1.DNSClusterFirstWithHostNet,
			ServiceAccountName: IPERF_BENCHMARK_SA,
			Containers: []corev1.Container{
				{
					Name:  IPERF_CLIENT_HOSTNETWORK_NAME,
					Image: IPERF_CLIENT_IMAGE,
					Env: []corev1.EnvVar{
						{Name: "BENCHMARK_MODE", Value: "host_network"},
						{Name: "TARGET_SVC", Value: HOSTNETWORK_HEADLESS_SERVICE_NAME},
						{Name: "TARGET_PORT", Value: fmt.Sprintf("%d", DEFAULT_IPERF_PORT)},
						{Name: "IPERF_PARALLEL", Value: fmt.Sprintf("%d", config.NumberOfConnections)},
						{Name: "IPERF_DURATION", Value: fmt.Sprintf("%d", config.DurationSeconds)},
						{Name: "EXCLUDE_NODES", Value: config.ExcludeNodes},
						{
							Name: "MY_NODE_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "spec.nodeName",
								},
							},
						},
						{Name: "VL_FULL_PATH", Value: "http://vlc-victoria-logs-cluster-vlinsert.victorialogs.svc.cluster.local:9481/insert/jsonline"},
						{Name: "VM_FULL_PATH", Value: "http://vminsert-vmks-victoria-metrics-k8s-stack.vmetrics.svc.cluster.local:8480/insert/0/prometheus/api/v1/import/prometheus"},
					},
					Command: []string{"/bin/bash", "/scripts/benchmark.sh"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "script-vol",
							MountPath: "/scripts",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "script-vol",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: IPERF_BENCHMARK_CONFIG_MAP,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Delete existing pod if it exists
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Delete(ctx, IPERF_CLIENT_HOSTNETWORK_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete existing pod '%s': %v\n", IPERF_CLIENT_HOSTNETWORK_NAME, err)
		}
	}

	// Wait briefly for deletion to complete
	time.Sleep(2 * time.Second)

	// Create the pod
	if _, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create pod '%s': %v\n", IPERF_CLIENT_HOSTNETWORK_NAME, err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] pod '%s' created for host network benchmark\n", IPERF_CLIENT_HOSTNETWORK_NAME)
	tw.Flush()
	return nil
}

// CreateIperfPodNetworkBenchmarkPod creates a Pod for running iperf benchmark over pod network
func CreateIperfPodNetworkBenchmarkPod(config BenchmarkConfig) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	defaultMode := int32(0755)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IPERF_CLIENT_PODNETWORK_NAME,
			Namespace: DEFAULT_NAMESPACE,
			Labels: map[string]string{
				"app":  "iperf-benchmark",
				"mode": "pod-network",
			},
		},
		Spec: corev1.PodSpec{
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
						{
							Weight: 100,
							Preference: corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "node-role.kubernetes.io/control-plane",
										Operator: corev1.NodeSelectorOpExists,
									},
								},
							},
						},
					},
				},
			},
			ServiceAccountName: IPERF_BENCHMARK_SA,
			Containers: []corev1.Container{
				{
					Name:  IPERF_CLIENT_PODNETWORK_NAME,
					Image: IPERF_CLIENT_IMAGE,
					Env: []corev1.EnvVar{
						{Name: "BENCHMARK_MODE", Value: "pod_network"},
						{Name: "TARGET_SVC", Value: "iperf-podnetwork"},
						{Name: "TARGET_PORT", Value: fmt.Sprintf("%d", DEFAULT_IPERF_PORT)},
						{Name: "IPERF_PARALLEL", Value: fmt.Sprintf("%d", config.NumberOfConnections)},
						{Name: "IPERF_DURATION", Value: fmt.Sprintf("%d", config.DurationSeconds)},
						{Name: "EXCLUDE_NODES", Value: config.ExcludeNodes},
						{
							Name: "MY_NODE_NAME",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "spec.nodeName",
								},
							},
						},
						{Name: "VL_FULL_PATH", Value: "http://vlc-victoria-logs-cluster-vlinsert.victorialogs.svc.cluster.local:9481/insert/jsonline"},
						{Name: "VM_FULL_PATH", Value: "http://vminsert-vmks-victoria-metrics-k8s-stack.vmetrics.svc.cluster.local:8480/insert/0/prometheus/api/v1/import/prometheus"},
					},
					Command: []string{"/bin/bash", "/scripts/benchmark.sh"},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "script-vol",
							MountPath: "/scripts",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "script-vol",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: IPERF_BENCHMARK_CONFIG_MAP,
							},
							DefaultMode: &defaultMode,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Delete existing pod if it exists
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Delete(ctx, IPERF_CLIENT_PODNETWORK_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete existing pod '%s': %v\n", IPERF_CLIENT_PODNETWORK_NAME, err)
		}
	}

	// Wait briefly for deletion to complete
	time.Sleep(2 * time.Second)

	// Create the pod
	if _, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create pod '%s': %v\n", IPERF_CLIENT_PODNETWORK_NAME, err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] pod '%s' created for pod network benchmark\n", IPERF_CLIENT_PODNETWORK_NAME)
	tw.Flush()
	return nil
}

// CreateIntraNodeBenchmarkPods creates individual pods on each node for intra-node benchmarks
func CreateIntraNodeBenchmarkPods(config BenchmarkConfig) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	defaultMode := int32(0755)

	// Get all nodes in the cluster
	nodes, err := K3sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %v", err)
	}

	// Build exclude nodes map
	excludeNodesMap := make(map[string]bool)
	if config.ExcludeNodes != "" {
		for _, node := range strings.Split(config.ExcludeNodes, ",") {
			excludeNodesMap[strings.TrimSpace(node)] = true
		}
	}

	// Delete existing intra-node benchmark pods
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: "app=iperf-benchmark,mode=intra-node",
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete existing intra-node pods: %v\n", err)
		}
	}
	time.Sleep(2 * time.Second)

	createdCount := 0
	for _, node := range nodes.Items {
		nodeName := node.Name

		// Skip excluded nodes
		if excludeNodesMap[nodeName] {
			fmt.Fprintf(tw, "[INFO] skipping excluded node '%s'\n", nodeName)
			tw.Flush()
			continue
		}

		// Check if node is schedulable
		if node.Spec.Unschedulable {
			fmt.Fprintf(tw, "[INFO] skipping unschedulable node '%s'\n", nodeName)
			tw.Flush()
			continue
		}

		podName := fmt.Sprintf("iperf-intra-node-%s", nodeName)
		intraNodeLabels := map[string]string{
			"app":  "iperf-benchmark",
			"mode": "intra-node",
			"node": nodeName,
		}

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: DEFAULT_NAMESPACE,
				Labels:    intraNodeLabels,
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: IPERF_BENCHMARK_SA,
				NodeName:           nodeName,
				Containers: []corev1.Container{
					{
						Name:  "iperf-client",
						Image: IPERF_CLIENT_IMAGE,
						Env: []corev1.EnvVar{
							{Name: "BENCHMARK_MODE", Value: "intra_node"},
							{Name: "INTRA_NODE_MODE", Value: "true"},
							{Name: "TARGET_SVC", Value: "iperf-podnetwork"},
							{Name: "TARGET_PORT", Value: fmt.Sprintf("%d", DEFAULT_IPERF_PORT)},
							{Name: "IPERF_PARALLEL", Value: fmt.Sprintf("%d", config.NumberOfConnections)},
							{Name: "IPERF_DURATION", Value: fmt.Sprintf("%d", config.DurationSeconds)},
							{Name: "EXCLUDE_NODES", Value: config.ExcludeNodes},
							{Name: "MY_NODE_NAME", Value: nodeName},
							{Name: "VL_FULL_PATH", Value: "http://vlc-victoria-logs-cluster-vlinsert.victorialogs.svc.cluster.local:9481/insert/jsonline"},
							{Name: "VM_FULL_PATH", Value: "http://vminsert-vmks-victoria-metrics-k8s-stack.vmetrics.svc.cluster.local:8480/insert/0/prometheus/api/v1/import/prometheus"},
						},
						Command: []string{"/bin/bash", "/scripts/benchmark.sh"},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "script-vol",
								MountPath: "/scripts",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "script-vol",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: IPERF_BENCHMARK_CONFIG_MAP,
								},
								DefaultMode: &defaultMode,
							},
						},
					},
				},
				RestartPolicy: corev1.RestartPolicyNever,
			},
		}

		if _, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
			fmt.Fprintf(tw, "[ERROR] failed to create intra-node pod on node '%s': %v\n", nodeName, err)
			tw.Flush()
			continue
		}
		createdCount++
		fmt.Fprintf(tw, "[INFO] created intra-node benchmark pod on node '%s'\n", nodeName)
		tw.Flush()
	}

	if createdCount == 0 {
		return fmt.Errorf("no intra-node benchmark pods were created")
	}

	fmt.Fprintf(tw, "[INFO] created %d intra-node benchmark pods\n", createdCount)
	tw.Flush()
	return nil
}

// WaitForIntraNodeBenchmarkCompletion waits for all intra-node benchmark pods to complete and collects results
func WaitForIntraNodeBenchmarkCompletion(config BenchmarkConfig, timeout time.Duration) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Fprintf(tw, "[INFO] waiting for intra-node benchmark pods to complete (timeout: %v)...\n", timeout)
	tw.Flush()

	// Wait for all intra-node benchmark pods to complete
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(tw, "[ERROR] timeout waiting for intra-node benchmark pods\n")
			tw.Flush()
			return fmt.Errorf("timeout waiting for intra-node benchmark pods")
		default:
			pods, err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).List(ctx, metav1.ListOptions{
				LabelSelector: "app=iperf-benchmark,mode=intra-node",
			})
			if err != nil {
				return fmt.Errorf("failed to list intra-node benchmark pods: %v", err)
			}

			if len(pods.Items) == 0 {
				time.Sleep(2 * time.Second)
				continue
			}

			allCompleted := true
			completedCount := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
					completedCount++
				} else {
					allCompleted = false
				}
			}

			if allCompleted {
				fmt.Fprintf(tw, "[INFO] all %d intra-node benchmark pods completed\n", len(pods.Items))
				tw.Flush()

				// Collect results from all pods
				for _, pod := range pods.Items {
					logs, err := GetPodLogs(pod.Name)
					if err != nil {
						fmt.Fprintf(tw, "[WARN] failed to get logs from pod %s: %v\n", pod.Name, err)
						continue
					}

					sourceNode := pod.Spec.NodeName
					report, err := ParseBenchmarkLogs(logs, config, "intra_node", sourceNode)
					if err != nil {
						fmt.Fprintf(tw, "[WARN] failed to parse logs from pod %s: %v\n", pod.Name, err)
						continue
					}

					if len(report.Results) > 0 {
						if err := CreateBenchmarkResultConfigMap(report, "intra-"+sourceNode); err != nil {
							fmt.Fprintf(tw, "[WARN] failed to save intra-node results for %s: %v\n", sourceNode, err)
						}
					}
				}

				return nil
			}

			fmt.Fprintf(tw, "[INFO] intra-node benchmark progress: %d/%d pods completed\n", completedCount, len(pods.Items))
			tw.Flush()
			time.Sleep(5 * time.Second)
		}
	}
}

func CreateNewBenchmark(config BenchmarkConfig) error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "[INFO] starting benchmark setup...\n")
	tw.Flush()

	// Create headless services
	if err := CreateIperfHeadlessServices(); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create headless services: %v\n", err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] headless services created successfully\n")
	tw.Flush()

	// Create DaemonSet for host network iperf servers
	if err := CreateIperfHostNetworkDaemonSet(); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create host network daemonset: %v\n", err)
		tw.Flush()
		return err
	}
	defer ReleaseBenchmarkResources()

	// Create DaemonSet for pod network iperf servers
	if err := CreateIperfPodNetworkDaemonSet(); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create pod network daemonset: %v\n", err)
		tw.Flush()
		return err
	}

	// wait for pods to be ready before creating benchmark pods
	fmt.Fprintf(tw, "[INFO] waiting for iperf server pods to be ready...\n")
	tw.Flush()
	if err := WaitForDaemonSetReady(IPERF_SERVERS_HN_DAEMONSET_NAME); err != nil {
		fmt.Fprintf(tw, "[ERROR] host network daemonset pods not ready: %v\n", err)
		tw.Flush()
		return err
	}
	if err := WaitForDaemonSetReady(IPERF_SERVERS_PN_DAEMONSET_NAME); err != nil {
		fmt.Fprintf(tw, "[ERROR] pod network daemonset pods not ready: %v\n", err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] iperf server host/pod networks daemonsets are ready\n")
	tw.Flush()

	// Create Roles and RoleBindings
	if err := CreateIperfRolesAndBindings(); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create roles and bindings: %v\n", err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] iperf roles and bindings created successfully\n")
	tw.Flush()

	// Create ConfigMap with benchmark script
	if err := CreateIperfBencharkScriptConfigMap(); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create benchmark script configmap: %v\n", err)
		tw.Flush()
		return err
	}
	fmt.Fprintf(tw, "[INFO] benchmark script configmap created successfully\n")
	tw.Flush()
	benchmarkTimeout := 20 * time.Minute

	// ========== HOST NETWORK BENCHMARK ==========
	fmt.Fprintf(tw, "[INFO] starting host network benchmark...\n")
	tw.Flush()

	if err := CreateIperfHostNetworkBenchmarkPod(config); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create host network benchmark pod: %v\n", err)
		tw.Flush()
		return err
	}

	// Get source node before waiting
	hostSourceNode, err := GetSourceNodeFromPod(IPERF_CLIENT_HOSTNETWORK_NAME)
	if err != nil {
		fmt.Fprintf(tw, "[WARN] failed to get source node for host network benchmark: %v\n", err)
		hostSourceNode = "unknown"
	}

	fmt.Fprintf(tw, "[INFO] waiting for host network benchmark to complete (timeout: %v)...\n", benchmarkTimeout)
	tw.Flush()

	hostLogs, err := WaitForPodCompletion(IPERF_CLIENT_HOSTNETWORK_NAME, benchmarkTimeout)
	if err != nil {
		fmt.Fprintf(tw, "[ERROR] host network benchmark failed: %v\n", err)
		tw.Flush()
		return err
	}

	// Parse and save host network results
	hostReport, err := ParseBenchmarkLogs(hostLogs, config, "host_network", hostSourceNode)
	if err != nil {
		fmt.Fprintf(tw, "[WARN] failed to parse host network benchmark logs: %v\n", err)
	} else {
		if err := CreateBenchmarkResultConfigMap(hostReport, "host"); err != nil {
			fmt.Fprintf(tw, "[WARN] failed to save host network benchmark results: %v\n", err)
		}
	}
	fmt.Fprintf(tw, "[INFO] host network benchmark completed with %d results\n", len(hostReport.Results))
	tw.Flush()
	ListBenchmarksWithFilter(context.Background(), ListFilter{
		Latest: true,
		Mode:   "host",
	})

	// ========== POD NETWORK BENCHMARK ==========
	fmt.Fprintf(tw, "[INFO] starting pod network benchmark...\n")
	tw.Flush()

	if err := CreateIperfPodNetworkBenchmarkPod(config); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create pod network benchmark pod: %v\n", err)
		tw.Flush()
		return err
	}

	// Get source node before waiting
	podSourceNode, err := GetSourceNodeFromPod(IPERF_CLIENT_PODNETWORK_NAME)
	if err != nil {
		fmt.Fprintf(tw, "[WARN] failed to get source node for pod network benchmark: %v\n", err)
		podSourceNode = "unknown"
	}

	fmt.Fprintf(tw, "[INFO] waiting for pod network benchmark to complete (timeout: %v)...\n", benchmarkTimeout)
	tw.Flush()

	podLogs, err := WaitForPodCompletion(IPERF_CLIENT_PODNETWORK_NAME, benchmarkTimeout)
	if err != nil {
		fmt.Fprintf(tw, "[ERROR] pod network benchmark failed: %v\n", err)
		tw.Flush()
		return err
	}

	// Parse and save pod network results
	podReport, err := ParseBenchmarkLogs(podLogs, config, "pod_network", podSourceNode)
	if err != nil {
		fmt.Fprintf(tw, "[WARN] failed to parse pod network benchmark logs: %v\n", err)
	} else {
		if err := CreateBenchmarkResultConfigMap(podReport, "pod"); err != nil {
			fmt.Fprintf(tw, "[WARN] failed to save pod network benchmark results: %v\n", err)
		}
	}
	fmt.Fprintf(tw, "[INFO] pod network benchmark completed with %d results\n", len(podReport.Results))
	tw.Flush()
	ListBenchmarksWithFilter(context.Background(), ListFilter{
		Latest: true,
		Mode:   "pod",
	})

	// ========== INTRA-NODE BENCHMARK ==========
	fmt.Fprintf(tw, "[INFO] starting intra-node benchmark...\n")
	tw.Flush()

	if err := CreateIntraNodeBenchmarkPods(config); err != nil {
		fmt.Fprintf(tw, "[ERROR] failed to create intra-node benchmark pods: %v\n", err)
		tw.Flush()
		return err
	}

	if err := WaitForIntraNodeBenchmarkCompletion(config, benchmarkTimeout); err != nil {
		fmt.Fprintf(tw, "[ERROR] intra-node benchmark failed: %v\n", err)
		tw.Flush()
		return err
	}

	fmt.Fprintf(tw, "[INFO] intra-node benchmark completed\n")
	tw.Flush()
	ListBenchmarksWithFilter(context.Background(), ListFilter{
		Latest: true,
		Mode:   "intra",
	})

	fmt.Fprintf(tw, "[INFO] all benchmarks completed successfully\n")
	tw.Flush()
	return nil
}

func WaitForDaemonSetReady(IPERF_SERVERS_PN_DAEMONSET_NAME string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for {
		ds, err := K3sClient.AppsV1().DaemonSets(DEFAULT_NAMESPACE).Get(ctx, IPERF_SERVERS_PN_DAEMONSET_NAME, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get daemonset '%s': %v", IPERF_SERVERS_PN_DAEMONSET_NAME, err)
		}

		if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}

func ReleaseBenchmarkResources() error {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Delete benchmark pods
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Delete(ctx, IPERF_CLIENT_HOSTNETWORK_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete pod '%s': %v\n", IPERF_CLIENT_HOSTNETWORK_NAME, err)
		}
	}
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).Delete(ctx, IPERF_CLIENT_PODNETWORK_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete pod '%s': %v\n", IPERF_CLIENT_PODNETWORK_NAME, err)
		}
	}

	// Delete intra-node benchmark pods by label selector
	if err := K3sClient.CoreV1().Pods(DEFAULT_NAMESPACE).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: "app=iperf-benchmark,mode=intra-node",
	}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete intra-node benchmark pods: %v\n", err)
		}
	}

	// Delete DaemonSets
	if err := K3sClient.AppsV1().DaemonSets(DEFAULT_NAMESPACE).Delete(ctx, IPERF_SERVERS_HN_DAEMONSET_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete daemonset '%s': %v\n", IPERF_SERVERS_HN_DAEMONSET_NAME, err)
		}
	}
	if err := K3sClient.AppsV1().DaemonSets(DEFAULT_NAMESPACE).Delete(ctx, IPERF_SERVERS_PN_DAEMONSET_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete daemonset '%s': %v\n", IPERF_SERVERS_PN_DAEMONSET_NAME, err)
		}
	}

	// Delete Services
	if err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Delete(ctx, PODNETWORK_HEADLESS_SERVICE_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete service '%s': %v\n", PODNETWORK_HEADLESS_SERVICE_NAME, err)
		}
	}
	if err := K3sClient.CoreV1().Services(DEFAULT_NAMESPACE).Delete(ctx, HOSTNETWORK_HEADLESS_SERVICE_NAME, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			fmt.Fprintf(tw, "[WARN] failed to delete service '%s': %v\n", HOSTNETWORK_HEADLESS_SERVICE_NAME, err)
		}
	}
	fmt.Fprintf(tw, "[INFO] benchmark resources cleanup completed\n")
	tw.Flush()
	return nil
}
