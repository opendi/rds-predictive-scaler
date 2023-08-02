package scaler

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"predictive-rds-scaler/history"
	"time"
)

const readerNamePrefix = "predictive-autoscaling-"

func New(config Config, awsSession *session.Session) (*Scaler, error) {
	rdsClient := rds.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	cloudWatchClient := cloudwatch.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	ctx := context.Background()
	var dynamoDbHistory, err = history.New(ctx, awsSession, config.AwsRegion, config.RdsClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB history: %v", err)
	}

	return &Scaler{
		config:           config,
		scaleOut:         Cooldown{},
		scaleIn:          Cooldown{},
		rdsClient:        rdsClient,
		cloudWatchClient: cloudWatchClient,
		dynamoDbHistory:  dynamoDbHistory,
	}, nil
}

func (s *Scaler) Run() {
	ticker := time.NewTicker(10 * time.Second)
	boostHours, err := parseBoostHours(s.config.BoostHours)
	if err != nil {
		fmt.Println("Error parsing scale out hours:", err)
		return
	}

	for range ticker.C {
		writerInstance, err := getWriterInstance(s.rdsClient, s.config.RdsClusterName)
		if err != nil {
			fmt.Println("Error getting writer instance:", err)
			continue
		}
		readerInstances, currentSize, err := getReaderInstances(s.rdsClient, s.config.RdsClusterName, StatusAll^StatusDeleting)
		if err != nil {
			fmt.Println("Error getting reader instances:", err)
			continue
		}
		cpuUtilization, err := s.getUtilization(readerInstances, writerInstance)
		if err != nil {
			fmt.Println("Error getting CPU utilization:", err)
			continue
		}

		minInstances := s.config.MinInstances
		if isBoostHour(time.Now().Hour(), boostHours) {
			minInstances = s.config.MinInstances + s.config.ScaleOutStep
		}

		fmt.Printf("Current CPU Utilization: %.2f%%, %d readers, in ScaleOut-Cooldown: %d, in ScaleIn-Cooldown: %d\n",
			cpuUtilization,
			currentSize,
			CalculateRemainingCooldown(s.config.ScaleOutCooldown, s.scaleOut.LastTime),
			CalculateRemainingCooldown(s.config.ScaleInCooldown, s.scaleIn.LastTime))

		if !s.scaleOut.InCooldown && ShouldScaleOut(cpuUtilization, s.config.TargetCpuUtil, currentSize, minInstances, s.config.MaxInstances) {
			scaleOutInstances := CalculateScaleOutInstances(s.config.MaxInstances, currentSize, s.config.ScaleOutStep)
			if scaleOutInstances > 0 {
				fmt.Printf("Scaling out by %d instances\n", scaleOutInstances)
				err := ScaleOut(s.rdsClient, s.config.RdsClusterName, readerNamePrefix, scaleOutInstances)
				if err != nil {
					fmt.Println("Error scaling out:", err)
				} else {
					s.scaleOut.InCooldown = true
					s.scaleOut.LastTime = time.Now()
					time.AfterFunc(s.config.ScaleOutCooldown, func() {
						s.scaleOut.InCooldown = false
					})
				}
			} else {
				fmt.Println("Max instances reached. Cannot scale out.")
			}
		}

		// Scale in if needed
		if !s.scaleIn.InCooldown && !s.scaleOut.InCooldown && ShouldScaleIn(cpuUtilization, s.config.TargetCpuUtil, currentSize, s.config.ScaleInStep, s.config.MinInstances) {
			scaleInInstances := CalculateScaleInInstances(currentSize, s.config.MinInstances, s.config.ScaleInStep)
			if scaleInInstances > 0 {
				fmt.Printf("Scaling in by %d instances\n", scaleInInstances)
				err := ScaleIn(s.rdsClient, s.config.RdsClusterName, scaleInInstances)
				if err != nil {
					fmt.Println("Error scaling in:", err)
				} else {
					s.scaleIn.InCooldown = true
					s.scaleIn.LastTime = time.Now()
					time.AfterFunc(s.config.ScaleInCooldown, func() {
						s.scaleIn.InCooldown = false
					})
				}
			} else {
				fmt.Println("Min instances reached. Cannot scale in.")
			}
		}
	}
}

func (s *Scaler) getUtilization(readerInstances []*rds.DBInstance, writerInstance *rds.DBInstance) (float64, error) {
	lastWeekTime := time.Now().Add(-time.Hour * 24 * 7).Add(time.Minute * 10) // last week, 10 minutes into the future
	lastWeekCpuUtilization, lastWeekCount, err := s.dynamoDbHistory.GetValue(lastWeekTime)
	if err != nil {
		return 0, fmt.Errorf("error getting historic value: %v", err)
	}

	currentCpuUtilization, currentActiveReaderCount, err := GetMaxCPUUtilization(readerInstances, writerInstance, s.cloudWatchClient)
	if err != nil {
		return 0, fmt.Errorf("error getting max CPU utilization: %v", err)
	}

	// Save the item to DynamoDB when scaling is required
	if currentActiveReaderCount > 0 || currentCpuUtilization > s.config.TargetCpuUtil {
		if err := s.dynamoDbHistory.SaveItem(currentActiveReaderCount, currentCpuUtilization); err != nil {
			return 0, fmt.Errorf("error saving item to DynamoDB: %v", err)
		}
	}

	if lastWeekCpuUtilization != 0 {
		interpolated := (lastWeekCpuUtilization * float64(currentActiveReaderCount+1)) / float64(lastWeekCount+1)
		if interpolated > currentCpuUtilization {
			currentCpuUtilization = interpolated
		}
	}

	return currentCpuUtilization, nil
}

func ScaleOut(rdsClient *rds.RDS, rdsClusterName string, readerNamePrefix string, numInstances uint) error {
	currentHour := time.Now().Hour()
	newReaderInstanceNames := make([]string, numInstances)

	for i := 0; i < int(numInstances); i++ {
		// Get the current writer instance
		writerInstance, err := getWriterInstance(rdsClient, rdsClusterName)
		if err != nil {
			return fmt.Errorf("failed to get current writer instance: %v", err)
		}

		// Generate a random UID for the new reader instance name
		randomUID := generateRandomUID()

		// Create the reader instance name with the prefix, current scale-out hour, and random UID
		readerName := fmt.Sprintf("%s%d-%s", readerNamePrefix, currentHour, randomUID)

		// Use the writer instance's configuration as a template for the new reader instance
		readerDBInstance := &rds.CreateDBInstanceInput{
			DBInstanceClass:         writerInstance.DBInstanceClass,
			Engine:                  writerInstance.Engine,
			DBClusterIdentifier:     aws.String(rdsClusterName),
			DBInstanceIdentifier:    aws.String(readerName),
			PubliclyAccessible:      aws.Bool(false),
			MultiAZ:                 writerInstance.MultiAZ,
			CopyTagsToSnapshot:      writerInstance.CopyTagsToSnapshot,
			AutoMinorVersionUpgrade: writerInstance.AutoMinorVersionUpgrade,
			DBParameterGroupName:    writerInstance.DBParameterGroups[0].DBParameterGroupName,
		}

		// Perform the scaling operation to add a reader to the cluster
		_, err = rdsClient.CreateDBInstance(readerDBInstance)
		if err != nil {
			return fmt.Errorf("failed to add reader instance: %v", err)
		}

		fmt.Printf("Scaling out operation successful. New reader instance name: %s\n", readerName)

		// Add the new reader instance name to the slice
		newReaderInstanceNames[i] = readerName
	}

	// Wait for all new reader instances to become "Available"
	fmt.Printf("Waiting for all new reader instances to become 'Available'...\n")
	err := waitForInstancesAvailable(rdsClient, newReaderInstanceNames)
	if err != nil {
		return fmt.Errorf("failed to wait for the new reader instances to become 'Available': %v", err)
	}

	fmt.Printf("All new reader instances are now 'Available'. Continuing...\n")
	return nil
}

func ScaleIn(rdsClient *rds.RDS, rdsClusterName string, numInstances uint) error {
	readerInstances, _, err := getReaderInstances(rdsClient, rdsClusterName, StatusAll)
	if err != nil {
		return fmt.Errorf("failed to get reader instances: %v", err)
	}

	for i := 0; i < int(numInstances); i++ {
		// Check if there are any reader instances available to scale in
		if len(readerInstances) == 0 {
			break
		}

		// Choose a reader instance to remove
		instance := readerInstances[0]

		// Check if the instance is in the process of deletion, and it's the last remaining reader instance
		if *instance.DBInstanceStatus == "deleting" && len(readerInstances) == 1 {
			fmt.Printf("The last remaining instance %s is already in status 'deleting'. Will not remove it to avoid service disruption.\n", *instance.DBInstanceIdentifier)
			break
		}

		// Skip over instances with the status "deleting"
		if *instance.DBInstanceStatus == "deleting" {
			fmt.Printf("Skipping instance %s already in status 'deleting'.\n", *instance.DBInstanceIdentifier)
			numInstances++
			readerInstances = readerInstances[1:]
			continue
		}

		// Wait for the instance to become deletable
		fmt.Printf("Waiting for the instance %s (status: %s) to become deletable...\n", *instance.DBInstanceIdentifier, *instance.DBInstanceStatus)
		err := waitUntilInstanceDeletable(rdsClient, *instance.DBInstanceIdentifier)
		if err != nil {
			return fmt.Errorf("failed to wait for instance to become deletable: %v", err)
		}

		// Remove the reader instance
		_, err = rdsClient.DeleteDBInstance(&rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: instance.DBInstanceIdentifier,
			SkipFinalSnapshot:    aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to remove reader instance: %v", err)
		}

		fmt.Printf("Scaling in operation successful. Removed reader instance: %s\n", *instance.DBInstanceIdentifier)

		// Remove the scaled-in instance from the list
		readerInstances = readerInstances[1:]
	}

	return nil
}

func waitUntilInstanceDeletable(rdsClient *rds.RDS, instanceIdentifier string) error {
	describeInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(instanceIdentifier),
	}

	for {
		describeOutput, err := rdsClient.DescribeDBInstances(describeInput)
		if err != nil {
			return fmt.Errorf("failed to describe RDS instance %s: %v", instanceIdentifier, err)
		}

		if len(describeOutput.DBInstances) == 0 {
			return fmt.Errorf("RDS instance %s not found", instanceIdentifier)
		}

		instanceStatus := *describeOutput.DBInstances[0].DBInstanceStatus
		if isDeletableStatus(instanceStatus) {
			fmt.Printf("Instance %s is now in deletable status (%s)\n", instanceIdentifier, instanceStatus)
			return nil
		}

		fmt.Printf("Waiting for instance %s to become deletable (current status: %s)...\n", instanceIdentifier, instanceStatus)
		time.Sleep(5 * time.Second)
	}
}

func isDeletableStatus(status string) bool {
	validStatuses := []string{"available", "backing-up", "creating"}
	return containsString(validStatuses, status)
}

// ShouldScaleOut returns true if scaling out is needed based on the current CPU utilization and the maximum number of instances.
func ShouldScaleOut(cpuUtilization, targetCpuUtil float64, currentSize, minInstances, maxInstances uint) bool {
	return currentSize < minInstances || (cpuUtilization > targetCpuUtil && currentSize < maxInstances)
}

// CalculateScaleOutInstances calculates the number of instances to scale out based on the maximum number of instances and the current size.
func CalculateScaleOutInstances(maxInstances, currentSize, scaleOutStep uint) uint {
	return minInt(scaleOutStep, maxInstances-currentSize)
}

// ShouldScaleIn returns true if scaling in is needed based on the current CPU utilization and the minimum number of instances.
func ShouldScaleIn(cpuUtilization float64, targetCpuUtil float64, currentSize, scaleInStep uint, minInstances uint) bool {
	if currentSize < minInstances+scaleInStep {
		fmt.Println("Scaling in not allowed, minimum instance threshold reached.")
		return false
	}

	if cpuUtilization < 50 && (currentSize-scaleInStep) <= 0 {
		fmt.Println("Scaling in required, CPU utilization is below 50% and would result in 0 instances.")
		return true
	}

	if cpuUtilization*(float64(currentSize)/float64(currentSize-scaleInStep)) <= targetCpuUtil {
		fmt.Println("Scaling in required, current load after scaling down is below the target CPU utilization.")
		return true
	}

	fmt.Println("No need to scale in.")
	return false
}

// CalculateScaleInInstances calculates the number of instances to scale in based on the current size and the minimum number of instances.
func CalculateScaleInInstances(currentSize, minInstances, scaleInStep uint) uint {
	return minInt(scaleInStep, currentSize-minInstances)
}
