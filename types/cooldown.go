package types

import "time"

type Cooldown struct {
	LastScale time.Time `json:"last_scale"`
	Timeout   time.Time `json:"timeout"`
	IsScaling bool      `json:"is_scaling"`
	Threshold uint
}
