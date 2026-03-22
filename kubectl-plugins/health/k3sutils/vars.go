package k3sutils

import (
	"github.com/ipfs/go-log/v2"
	"k8s.io/client-go/kubernetes"
)

var logger = log.Logger("health")
var CONFIG_MAP_PREFIX string = "iperf-result-"
var DEFAULT_APP_CONTAINER_NAME string = "iperf"
var IPERF_CLIENT_IMAGE string = "katakuri100/kube-iperf3-client:v0.2"
var IPERF_SERVER_IMAGE string = "networkstatic/iperf3:latest"
var IPERF_SERVERS_PN_DAEMONSET_NAME string = "iperf-servers-podnetwork"
var IPERF_SERVERS_HN_DAEMONSET_NAME string = "iperf-servers-hostnetwork"
var HOSTNETWORK_HEADLESS_SERVICE_NAME string = "iperf-hostnetwork"
var PODNETWORK_HEADLESS_SERVICE_NAME string = "iperf-podnetwork"
var DEFAULT_NAMESPACE string = "health-monitoring"
var DEFAULT_IPERF_PORT = 5201
var IPERF_BENCHMARK_SA string = "iperf-benchmark-sa"
var IPERF_NODE_POD_READER_ROLE string = "iperf-node-pod-reader"
var IPERF_BENCHMARK_ROLE_BINDING string = "iperf-benchmark-binding"
var IPERF_BENCHMARK_CONFIG_MAP string = "iperf-benchmark-script"
var IPERF_CLIENT_PODNETWORK_NAME string = "iperf-client-pod"
var IPERF_CLIENT_HOSTNETWORK_NAME string = "iperf-client-host"

var K3sClient *kubernetes.Clientset
