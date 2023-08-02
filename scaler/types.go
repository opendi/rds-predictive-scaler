package scaler

import (
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"predictive-rds-scaler/history"
	"time"
)

type Config struct {
	AwsRegion        string
	RdsClusterName   string
	MaxInstances     uint
	MinInstances     uint
	BoostHours       string
	TargetCpuUtil    float64
	ScaleOutCooldown time.Duration
	ScaleInCooldown  time.Duration
	ScaleInStep      uint
	ScaleOutStep     uint
}

type Scaler struct {
	config           Config
	scaleOut         Cooldown
	scaleIn          Cooldown
	rdsClient        *rds.RDS
	cloudWatchClient *cloudwatch.CloudWatch
	dynamoDbHistory  *history.History
}

type Cooldown struct {
	InCooldown bool
	LastTime   time.Time
}
