package scaler

import (
	"github.com/rs/zerolog"
	"time"

	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"predictive-rds-scaler/history"
)

type Config struct {
	AwsRegion        string
	RdsClusterName   string
	MaxInstances     uint
	MinInstances     uint
	BoostHours       string
	TargetCpuUtil    float64
	PlanAheadTime    time.Duration
	ScaleOutCooldown time.Duration
	ScaleInCooldown  time.Duration
	ScaleInStep      uint
	ScaleOutStep     uint
	ServerPort       uint
}

type Scaler struct {
	config           Config
	scaleOutStatus   Cooldown
	scaleInStatus    Cooldown
	rdsClient        *rds.RDS
	cloudWatchClient *cloudwatch.CloudWatch
	dynamoDbHistory  *history.History
	logger           *zerolog.Logger
	broadcast        chan Broadcast
}

type Cooldown struct {
	LastScale time.Time `json:"last_scale"`
	Timeout   time.Time `json:"timeout"`
	IsScaling bool      `json:"is_scaling"`
	threshold uint
}

type Broadcast struct {
	MessageType string      `json:"type"`
	Data        interface{} `json:"data"`
}
