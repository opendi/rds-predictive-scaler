package main

import (
	"flag"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/rs/zerolog/log"
	"predictive-rds-scaler/logutil"
	"predictive-rds-scaler/scaler"
	"time"
)

var config scaler.Config

func init() {
	flag.UintVar(&config.ScaleInStep, "scaleInStep", 1, "Number of reader instances to scale in at a time")
	flag.UintVar(&config.ScaleOutStep, "scaleOutStep", 1, "Number of reader instances to scale out at a time")
	flag.UintVar(&config.MinInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.UintVar(&config.MaxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")
	flag.StringVar(&config.AwsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&config.BoostHours, "boostHours", "", "Comma-separated list of hours to boost minInstances")
	flag.StringVar(&config.RdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.Float64Var(&config.TargetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.DurationVar(&config.ScaleOutCooldown, "scaleOutCooldown", 10*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")
	flag.DurationVar(&config.ScaleInCooldown, "scaleInCooldown", 5*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")

	flag.Parse()
}

func main() {
	logutil.InitLogger()

	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create AWS session")
	}

	// Create the logger
	logger := logutil.GetLogger()

	rdsScaler, err := scaler.New(config, logger, awsSession)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create scaler")
	}

	rdsScaler.Run()
}
