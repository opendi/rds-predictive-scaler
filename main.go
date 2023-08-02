package main

import (
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"log"
	"predictive-rds-scaler/scaler"
	"time"
)

var (
	awsRegion        string
	rdsClusterName   string
	maxInstances     uint
	minInstances     uint
	boostHours       string
	targetCpuUtil    float64
	scaleOutCooldown time.Duration
	scaleInCooldown  time.Duration
	scaleInStep      uint
	scaleOutStep     uint
)

func init() {
	flag.StringVar(&awsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&rdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.UintVar(&maxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")
	flag.UintVar(&minInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.StringVar(&boostHours, "boostHours", "", "Comma-separated list of hours to boost minInstances")
	flag.Float64Var(&targetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.DurationVar(&scaleOutCooldown, "scaleOutCooldown", 10*60, "Cooldown time after scaling actions to avoid constant scale up/down activity (in seconds)")
	flag.DurationVar(&scaleInCooldown, "scaleInCooldown", 5*60, "Cooldown time after scaling actions to avoid constant scale up/down activity (in seconds)")
	flag.UintVar(&scaleInStep, "scaleInStep", 1, "Number of reader instances to scale in at a time")
	flag.UintVar(&scaleOutStep, "scaleOutStep", 1, "Number of reader instances to scale out at a time")
	flag.Parse()

	if awsRegion == "" || rdsClusterName == "" {
		fmt.Println("Please provide values for awsRegion and rdsClusterName.")
		return
	}

	awsSession, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		log.Fatalf("Failed to create AWS awsSession: %v", err)
	}

	scaler.Init(awsSession, awsRegion, rdsClusterName, maxInstances, minInstances, boostHours, targetCpuUtil, scaleOutCooldown, scaleInCooldown, scaleInStep, scaleOutStep)
}

func main() {
	scaler.Run()
}
