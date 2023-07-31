package main

import (
	"flag"
	"fmt"
	"log"
	"predictive-rds-scaler/scaler"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
)

const readerNamePrefix = "predictive-autoscaling-"

var (
	awsRegion      string
	rdsClusterName string
	maxInstances   = 5
	minInstances   = 1
	targetCpuUtil  = 75.0
	scaleCooldown  = 10 * time.Minute
	scaleInStep    = 1
	scaleOutStep   = 1
)

var inCooldown bool

func init() {
	flag.StringVar(&awsRegion, "awsRegion", "", "AWS region")
	flag.StringVar(&rdsClusterName, "rdsClusterName", "", "RDS cluster name")
	flag.IntVar(&maxInstances, "maxInstances", 5, "Maximum number of readers allowed in the cluster")
	flag.IntVar(&minInstances, "minInstances", 2, "Minimum number of readers required in the cluster")
	flag.Float64Var(&targetCpuUtil, "targetCpuUtilization", 70.0, "Target CPU utilization percentage")
	flag.DurationVar(&scaleCooldown, "scaleCooldown", 10*time.Minute, "Cooldown time after scaling actions to avoid constant scale up/down activity")
	flag.IntVar(&scaleInStep, "scaleInStep", 1, "Number of reader instances to scale in at a time")
	flag.IntVar(&scaleOutStep, "scaleOutStep", 1, "Number of reader instances to scale out at a time")
	flag.Parse()
}

func main() {
	if awsRegion == "" || rdsClusterName == "" {
		fmt.Println("Please provide values for awsRegion and rdsClusterName.")
		return
	}

	// Create an AWS session
	sess, err := session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	})

	if err != nil {
		log.Fatalf("Failed to create AWS session: %v", err)
	}

	// Create an RDS service client
	rdsClient := rds.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	cloudWatchClient := cloudwatch.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	ticker := time.NewTicker(10 * time.Second)

	for range ticker.C {
		// Determine the current CPU utilization
		cpuUtilization, err := scaler.GetMaxCPUUtilization(rdsClient, cloudWatchClient, rdsClusterName)
		if err != nil {
			fmt.Println("Error:", err)
			continue
		}

		fmt.Printf("Current CPU Utilization: %.2f%%\n", cpuUtilization)

		// Scale out if needed
		readerInstances := scaler.GetReaderInstances(rdsClient, rdsClusterName)
		currentSize := len(readerInstances)

		if !inCooldown && scaler.ShouldScaleOut(cpuUtilization, targetCpuUtil, currentSize+scaleOutStep, maxInstances) {
			scaleOutInstances := scaler.CalculateScaleOutInstances(maxInstances, currentSize, scaleOutStep)
			if scaleOutInstances > 0 {
				fmt.Printf("Scaling out by %d instances\n", scaleOutInstances)
				err := scaler.ScaleOut(rdsClient, rdsClusterName, readerNamePrefix, scaleOutInstances)
				if err != nil {
					fmt.Println("Error scaling out:", err)
				} else {
					inCooldown = true
					time.AfterFunc(scaleCooldown, func() {
						inCooldown = false
					})
				}
			} else {
				fmt.Println("Max instances reached. Cannot scale out.")
			}
		}

		// Scale in if needed
		if !inCooldown && scaler.ShouldScaleIn(cpuUtilization, targetCpuUtil, currentSize, minInstances) {
			scaleInInstances := scaler.CalculateScaleInInstances(currentSize, minInstances, scaleInStep)
			if scaleInInstances > 0 {
				fmt.Printf("Scaling in by %d instances\n", scaleInInstances)
				err := scaler.ScaleIn(rdsClient, rdsClusterName, scaleInInstances)
				if err != nil {
					fmt.Println("Error scaling in:", err)
				} else {
					inCooldown = true
					time.AfterFunc(scaleCooldown, func() {
						inCooldown = false
					})
				}
			} else {
				fmt.Println("Min instances reached. Cannot scale in.")
			}
		}
	}
}
