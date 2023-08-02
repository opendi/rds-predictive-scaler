package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"predictive-rds-scaler/scaler"
	"time"
)

var scalerConfig scaler.ScalerConfig

func init() {
	flag.UintVar(&scalerConfig.ScaleInStep, "scaleInStep", 1, "Number of reader instances to scale in at a time")
	flag.UintVar(&scalerConfig.ScaleOutStep, "scaleOutStep", 1, "Number of reader instances to scale out at a time")
	flag.UintVar(&scalerConfig.MinInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.UintVar(&scalerConfig.MaxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")
	flag.StringVar(&scalerConfig.AwsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&scalerConfig.BoostHours, "boostHours", "", "Comma-separated list of hours to boost minInstances")
	flag.StringVar(&scalerConfig.RdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.Float64Var(&scalerConfig.TargetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.DurationVar(&scalerConfig.ScaleOutCooldown, "scaleOutCooldown", 10*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")
	flag.DurationVar(&scalerConfig.ScaleInCooldown, "scaleInCooldown", 5*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")

	flag.Parse()
}

func main() {
	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		log.Fatal("Failed to create AWS session:", err)
	}

	err = scaler.Init(awsSession, scalerConfig)
	if err != nil {
		log.Fatal("Failed to initialize scaler:", err)
	}

	scaler.Run()
}
