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
	config ScalerConfig

	inScaleOutCooldown bool
	lastScaleOutTime   time.Time

	inScaleInCooldown    bool
	lastScaleInTime      time.Time
	originalMinInstances uint
	rdsClient            *rds.RDS
	cloudWatchClient     *cloudwatch.CloudWatch
	dynamoDBHistory      *history.DynamoDBHistory
)

type ScalerConfig struct {
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

func Init(sess *session.Session, scalerConfig ScalerConfig) error {
	config = scalerConfig
	originalMinInstances = config.MinInstances

	// Create an RDS service client
	rdsClient = rds.New(sess, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	// Create a CloudWatch service client
	cloudWatchClient = cloudwatch.New(sess, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	// Create a DynamoDB service client
	dynamoDBClient := dynamodb.New(sess, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	// Create the DynamoDBHistory instance
	var err error
	dynamoDBHistory, err = history.New(dynamoDBClient, config.RdsClusterName)
	if err != nil {
		return fmt.Errorf("failed to create DynamoDBHistory: %v", err)
	}

	return nil
}

func Run() {
	ticker := time.NewTicker(10 * time.Second)
	scaleOutHours, _ := ParseScaleOutHours(config.BoostHours)

	for range ticker.C {
		writerInstance, _ := GetWriterInstance(rdsClient, config.RdsClusterName)
		readerInstances, currentSize := GetReaderInstances(rdsClient, config.RdsClusterName, StatusAll^StatusDeleting)

		cpuUtilization := getUtilization(readerInstances, writerInstance)

		if IsScaleOutHour(time.Now().Hour(), scaleOutHours) {
			config.MinInstances = originalMinInstances + 1
		} else {
			config.MinInstances = originalMinInstances
		}

		fmt.Printf("Current CPU Utilization: %.2f%%, %d readers, in ScaleOut-Cooldown: %d, in ScaleIn-Cooldown: %d\n",
			cpuUtilization,
			currentSize,
			CalculateRemainingCooldown(config.ScaleOutCooldown, lastScaleOutTime),
			CalculateRemainingCooldown(config.ScaleInCooldown, lastScaleInTime))

		if !inScaleOutCooldown && ShouldScaleOut(cpuUtilization, config.TargetCpuUtil, currentSize, config.MinInstances, config.MaxInstances) {
			scaleOutInstances := CalculateScaleOutInstances(config.MaxInstances, currentSize, config.ScaleOutStep)
			if scaleOutInstances > 0 {
				fmt.Printf("Scaling out by %d instances\n", scaleOutInstances)
				err := ScaleOut(rdsClient, config.RdsClusterName, readerNamePrefix, scaleOutInstances)
				if err != nil {
					fmt.Println("Error scaling out:", err)
				} else {
					inScaleOutCooldown = true
					lastScaleOutTime = time.Now()
					time.AfterFunc(config.ScaleOutCooldown, func() {
						inScaleOutCooldown = false
					})
				}
			} else {
				fmt.Println("Max instances reached. Cannot scale out.")
			}
		}

		// Scale in if needed
		if !inScaleInCooldown && !inScaleOutCooldown && ShouldScaleIn(cpuUtilization, config.TargetCpuUtil, currentSize, config.ScaleInStep, config.MinInstances) {
			scaleInInstances := CalculateScaleInInstances(currentSize, config.MinInstances, config.ScaleInStep)
			if scaleInInstances > 0 {
				fmt.Printf("Scaling in by %d instances\n", scaleInInstances)
				err := ScaleIn(rdsClient, config.RdsClusterName, scaleInInstances)
				if err != nil {
					fmt.Println("Error scaling in:", err)
				} else {
					inScaleInCooldown = true
					lastScaleInTime = time.Now()
					time.AfterFunc(config.ScaleInCooldown, func() {
						inScaleInCooldown = false
					})
				}
			} else {
				fmt.Println("Min instances reached. Cannot scale in.")
			}
		}
	}
}

func getUtilization(readerInstances []*rds.DBInstance, writerInstance *rds.DBInstance) float64 {
	lastWeekTime := time.Now().Add(-time.Hour * 24 * 7).Add(time.Minute * 10) // last week, 10 minutes into the future
	lastWeekCpuUtilization, lastWeekCount, err := dynamoDBHistory.GetHistoricValue(lastWeekTime)
	if err != nil {
		fmt.Println("Error:", err)
	}

	currentCpuUtilization, currentActiveReaderCount, err := GetMaxCPUUtilization(readerInstances, writerInstance, cloudWatchClient)
	if err != nil {
		fmt.Println("Error:", err)
		currentCpuUtilization = 100
	}

	// Save the item to DynamoDB when scaling is required
	if currentActiveReaderCount > 0 || currentCpuUtilization > config.TargetCpuUtil {
		if err := dynamoDBHistory.SaveItem(currentActiveReaderCount, currentCpuUtilization); err != nil {
			fmt.Println("Error saving item to DynamoDB:", err)
		}
	}

	if lastWeekCpuUtilization != 0 {
		interpolated := (lastWeekCpuUtilization * float64(currentActiveReaderCount+1)) / float64(lastWeekCount+1)
		if interpolated > currentCpuUtilization {
			currentCpuUtilization = interpolated
		}
	}

	return currentCpuUtilization
}
