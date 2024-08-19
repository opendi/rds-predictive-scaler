package types

import "time"

type ClusterStatus struct {
	Identifier            string           `json:"identifier"`
	Timestamp             time.Time        `json:"timestamp"`
	AverageCPUUtilization float64          `json:"average_cpu_utilization"`
	CurrentActiveReaders  uint             `json:"current_active_readers"`
	OptimalSize           uint             `json:"optimal_size"`
	Instances             []InstanceStatus `json:"instance_status"`
}
