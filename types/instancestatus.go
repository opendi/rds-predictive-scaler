package types

type InstanceStatus struct {
	Identifier     string  `json:"identifier"`
	IsWriter       bool    `json:"is_writer"`
	Status         string  `json:"status"`
	CPUUtilization float64 `json:"cpu_utilization"`
}
