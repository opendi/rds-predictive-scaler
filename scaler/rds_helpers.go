package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"math/rand"
	"strings"
)

func getCurrentWriterInstance(rdsClient *rds.RDS, rdsClusterName string) (*rds.DBInstance, error) {
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
		return nil, fmt.Errorf("failed to describe RDS instances: %v", err)
	}

	if len(describeOutput.DBInstances) == 0 {
		return nil, fmt.Errorf("no matching RDS cluster found: %s", rdsClusterName)
	}

	// Find the writer instance in the cluster members
	for _, instance := range describeOutput.DBInstances {
		if aws.StringValue(instance.DBClusterIdentifier) == rdsClusterName {
			return instance, nil
		}
	}

	return nil, fmt.Errorf("writer instance not found in the RDS cluster")
}

func hasRunningReaderForHour(rdsClient *rds.RDS, rdsClusterName string, readerNamePrefix string, hour int) (bool, error) {
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
		return false, fmt.Errorf("failed to describe RDS instances: %v", err)
	}

	// Check if any reader instance with the desired name prefix exists for the current scale-out hour
	for _, instance := range describeOutput.DBInstances {
		if strings.HasPrefix(*instance.DBInstanceIdentifier, fmt.Sprintf("%s%d-", readerNamePrefix, hour)) {
			// Check if the instance is in one of the running statuses
			runningStatuses := []string{"available", "backing-up", "creating"}
			for _, status := range runningStatuses {
				if *instance.DBInstanceStatus == status {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func generateRandomUID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	uid := make([]byte, 8)
	for i := range uid {
		uid[i] = charset[rand.Intn(len(charset))]
	}
	return string(uid)
}
