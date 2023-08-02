package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/rds"
	"predictive-rds-scaler/history"
	"time"
)

const readerNamePrefix = "predictive-autoscaling-"

var (
	awsRegion            string
	rdsClusterName       string
	maxInstances         uint
	minInstances         uint
	originalMinInstances uint
	boostHours           string
	targetCpuUtil        float64
	scaleOutCooldown     time.Duration
	scaleInCooldown      time.Duration
	scaleInStep          uint
	scaleOutStep         uint
	inScaleOutCooldown   bool
	lastScaleOutTime     time.Time

	inScaleInCooldown bool
	lastScaleInTime   time.Time

	rdsClient        *rds.RDS
	cloudWatchClient *cloudwatch.CloudWatch
	dynamoDBHistory  *history.DynamoDBHistory
)

func Init(sess *session.Session, region, clusterName string, maxInst, minInst uint, boostHrs string, targetCpuU float64, scaleOutCD, scaleInCD time.Duration, scaleInSt, scaleOutSt uint) {
	awsRegion = region
	rdsClusterName = clusterName
	maxInstances = maxInst
	minInstances = minInst
	originalMinInstances = minInst
	boostHours = boostHrs
	targetCpuUtil = targetCpuU
	scaleOutCooldown = scaleOutCD
	scaleInCooldown = scaleInCD
	scaleInStep = scaleInSt
	scaleOutStep = scaleOutSt

	// Create an RDS service client
	rdsClient = rds.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	// Create a CloudWatch service client
	cloudWatchClient = cloudwatch.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	// Create a DynamoDB service client
	dynamoDBClient := dynamodb.New(sess, &aws.Config{
		Region: aws.String(awsRegion),
	})

	// Create the DynamoDBHistory instance
	dynamoDBHistory, _ = history.New(dynamoDBClient, rdsClusterName)
}

func Run() {
	ticker := time.NewTicker(10 * time.Second)
	scaleOutHours, _ := ParseScaleOutHours(boostHours)

	for range ticker.C {
		_, currentSize := GetReaderInstances(rdsClient, rdsClusterName, StatusAll^StatusDeleting)
		cpuUtilization := getUtilization(currentSize)

		if IsScaleOutHour(time.Now().Hour(), scaleOutHours) {
			minInstances = originalMinInstances + 1
		} else {
			minInstances = originalMinInstances
		}

		fmt.Printf("Current CPU Utilization: %.2f%%, %d readers, in ScaleOut-Cooldown: %d, in ScaleIn-Cooldown: %d\n",
			cpuUtilization,
			currentSize,
			CalculateRemainingCooldown(scaleOutCooldown, lastScaleOutTime),
			CalculateRemainingCooldown(scaleInCooldown, lastScaleInTime))

		if !inScaleOutCooldown && ShouldScaleOut(cpuUtilization, targetCpuUtil, currentSize, minInstances, maxInstances) {
			scaleOutInstances := CalculateScaleOutInstances(maxInstances, currentSize, scaleOutStep)
			if scaleOutInstances > 0 {
				fmt.Printf("Scaling out by %d instances\n", scaleOutInstances)
				err := ScaleOut(rdsClient, rdsClusterName, readerNamePrefix, scaleOutInstances)
				if err != nil {
					fmt.Println("Error scaling out:", err)
				} else {
					inScaleOutCooldown = true
					lastScaleOutTime = time.Now()
					time.AfterFunc(scaleOutCooldown, func() {
						inScaleOutCooldown = false
					})
				}
			} else {
				fmt.Println("Max instances reached. Cannot scale out.")
			}
		}

		// Scale in if needed
		if !inScaleInCooldown && !inScaleOutCooldown && ShouldScaleIn(cpuUtilization, targetCpuUtil, currentSize, scaleInStep, minInstances) {
			scaleInInstances := CalculateScaleInInstances(currentSize, minInstances, scaleInStep)
			if scaleInInstances > 0 {
				fmt.Printf("Scaling in by %d instances\n", scaleInInstances)
				err := ScaleIn(rdsClient, rdsClusterName, scaleInInstances)
				if err != nil {
					fmt.Println("Error scaling in:", err)
				} else {
					inScaleInCooldown = true
					lastScaleInTime = time.Now()
					time.AfterFunc(scaleInCooldown, func() {
						inScaleInCooldown = false
					})
				}
			} else {
				fmt.Println("Min instances reached. Cannot scale in.")
			}
		}
	}
}

func getUtilization(currentSize uint) float64 {
	lastWeekTime := time.Now().Add(-time.Hour * 24 * 7).Add(time.Minute * 10) // last week, 10 minutes into the future
	lastWeekCpuUtilization, err := dynamoDBHistory.GetMaxCpuUtilization(lastWeekTime)
	if err != nil {
		fmt.Println("Error:", err)
	}
	cpuUtilization, err := GetMaxCPUUtilization(rdsClient, cloudWatchClient, rdsClusterName)
	if err != nil {
		fmt.Println("Error:", err)
		cpuUtilization = 100
	} else {
		if dynamoDBHistory.SaveItem(time.Now().Format(time.RFC3339), currentSize, cpuUtilization) != nil {
			fmt.Println("Error saving item to DynamoDB:", err)
		}
	}

	if lastWeekCpuUtilization != 0 {
		cpuUtilization = lastWeekCpuUtilization
	}
	return cpuUtilization
}
