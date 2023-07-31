package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"math/rand"
)

func GetCurrentWriterInstance(rdsClient *rds.RDS, rdsClusterName string) (*rds.DBInstance, error) {
	describeInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(rdsClusterName),
	}

	clusterOutput, err := rdsClient.DescribeDBClusters(describeInput)
	if err != nil {
		return nil, err
	}

	if len(clusterOutput.DBClusters) == 0 {
		return nil, fmt.Errorf("aurora cluster not found: %s", rdsClusterName)
	}

	// Loop through the cluster members to find the writer instance
	for _, member := range clusterOutput.DBClusters[0].DBClusterMembers {
		if aws.BoolValue(member.IsClusterWriter) {
			describeInstanceInput := &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: member.DBInstanceIdentifier,
			}

			instanceOutput, err := rdsClient.DescribeDBInstances(describeInstanceInput)
			if err != nil {
				return nil, err
			}

			if len(instanceOutput.DBInstances) > 0 {
				return instanceOutput.DBInstances[0], nil
			}

			return nil, fmt.Errorf("Writer instance not found in cluster: %s", rdsClusterName)
		}
	}

	return nil, fmt.Errorf("Writer instance not found in cluster: %s", rdsClusterName)
}

func GetReaderInstances(rdsClient *rds.RDS, rdsClusterName string) []*rds.DBInstance {
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
		fmt.Println("Error describing RDS instances:", err)
		return nil
	}

	writerInstance, err := GetCurrentWriterInstance(rdsClient, rdsClusterName)
	if err != nil {
		fmt.Println("Error describing RDS writer instance:", err)
		return nil
	}

	readerInstances := make([]*rds.DBInstance, 0)
	for _, instance := range describeOutput.DBInstances {
		if aws.StringValue(writerInstance.DBInstanceIdentifier) != aws.StringValue(instance.DBInstanceIdentifier) {
			readerInstances = append(readerInstances, instance)
		}
	}

	return readerInstances
}

func generateRandomUID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	uid := make([]byte, 8)
	for i := range uid {
		uid[i] = charset[rand.Intn(len(charset))]
	}
	return string(uid)
}
