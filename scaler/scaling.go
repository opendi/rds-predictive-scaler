package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"strings"
	"time"
)

func ScaleOut(rdsClient *rds.RDS, rdsClusterName string, readerNamePrefix string) error {
	currentHour := time.Now().Hour()

	// Check if there is already a running reader instance for the current scale-out hour
	hasRunningReader, err := hasRunningReaderForHour(rdsClient, rdsClusterName, readerNamePrefix, currentHour)
	if err != nil {
		return fmt.Errorf("failed to check for running reader instance: %v", err)
	}

	if hasRunningReader {
		return fmt.Errorf("a running reader instance already exists for the current scale-out hour (%d)", currentHour)
	}

	// Get the current writer instance
	writerInstance, err := getCurrentWriterInstance(rdsClient, rdsClusterName)
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
	}

	// Perform the scaling operation to add a reader to the cluster
	_, err = rdsClient.CreateDBInstance(readerDBInstance)
	if err != nil {
		return fmt.Errorf("failed to add reader instance: %v", err)
	}

	fmt.Printf("Scaling out operation successful. New reader instance name: %s\n", readerName)
	return nil
}

func ScaleIn(rdsClient *rds.RDS, rdsClusterName string, readerNamePrefix string) error {
	// Describe the RDS cluster to get the list of reader instances
	describeInput := &rds.DescribeDBInstancesInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-cluster-id"),
				Values: []*string{aws.String(rdsClusterName)},
			},
		},
	}

	describeOutput, err := rdsClient.DescribeDBInstances(describeInput)
	if err != nil {
		return fmt.Errorf("failed to describe RDS instances: %v", err)
	}

	// Filter reader instances with the given prefix
	readerInstances := make([]*rds.DBInstance, 0)
	for _, instance := range describeOutput.DBInstances {
		if strings.HasPrefix(*instance.DBInstanceIdentifier, readerNamePrefix) {
			readerInstances = append(readerInstances, instance)
		}
	}

	// Choose a reader instance to remove (if any)
	for _, instance := range readerInstances {
		// Skip over instances with the status "deleting"
		if *instance.DBInstanceStatus == "deleting" {
			fmt.Printf("Skipping instance %s already in status 'deleting'.\n", *instance.DBInstanceIdentifier)
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
	}

	if len(readerInstances) == 0 {
		fmt.Println("Cannot scale in. No reader instance with the given prefix found.")
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
		time.Sleep(30 * time.Second)
	}
}

func isDeletableStatus(status string) bool {
	validStatuses := []string{"available", "backing-up", "creating"}
	return containsString(validStatuses, status)
}

func containsString(list []string, str string) bool {
	for _, s := range list {
		if s == str {
			return true
		}
	}
	return false
}
