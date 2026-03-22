package k3sutils

import "time"

type ConfigMapSetRow struct {
	Name      string
	Namespace string
	Ready     int32
	Age       time.Duration
}

type BenchmarkConfig struct {
	NumberOfConnections int
	DurationSeconds     int
}

type IperfTestResult struct {
	Destination   string  `json:"destination"`
	DestNode      string  `json:"dest_node"`
	Protocol      string  `json:"protocol"`  // tcp / udp
	Direction     string  `json:"direction"` // UL / DL
	ThroughputBps float64 `json:"throughput_bps"`
	Retransmits   int64   `json:"retransmits,omitempty"`    // TCP only
	AvgCWND       float64 `json:"avg_cwnd_bytes,omitempty"` // TCP only
	RTTMin        float64 `json:"rtt_min_ms,omitempty"`
	RTTAvg        float64 `json:"rtt_avg_ms,omitempty"`
	RTTMax        float64 `json:"rtt_max_ms,omitempty"`
	JitterMs      float64 `json:"jitter_ms,omitempty"`       // UDP only
	PacketLossPct float64 `json:"packet_loss_pct,omitempty"` // UDP only
}

type BenchmarkReport struct {
	Timestamp     string            `json:"timestamp"`
	SourceNode    string            `json:"source_node"`
	BenchmarkMode string            `json:"benchmark_mode"` // e.g., host_network
	Parallel      int               `json:"parallel"`
	Duration      int               `json:"duration_sec"`
	Results       []IperfTestResult `json:"results"`
}

type DestNodeSummary struct {
	DestNode      string
	LatestRun     time.Time
	TotalTests    int
	HostModeTests int
	PodModeTests  int
	AvgThroughput float64 // Average throughput
	AvgRTT        float64 // Average RTT in ms (TCP)
	AvgJitter     float64 // Average jitter in ms (UDP)
	AvgPacketLoss float64 // Average packet loss % (UDP)
	TotalRetrans  int64   // Total retransmits (TCP)
}

type DestNodeTestResult struct {
	Timestamp     time.Time
	SourceNode    string
	DestNode      string
	BenchmarkMode string
	Protocol      string
	Direction     string
	ThroughputBps float64
	RTTAvg        float64
	JitterMs      float64
	PacketLossPct float64
	Retransmits   int64
}

type ListFilter struct {
	Latest bool   // Show only the latest result per dest node
	Today  bool   // Show only today's results
	Mode   string // Filter by mode: "host", "pod", or "" for all
	Node   string // Filter by destination node
	Limit  int    // Maximum number of results to show
}