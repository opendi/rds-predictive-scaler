package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"math/rand"
	"time"
)

const (
	StatusAll                                          = 0xFFFFFFFFFFFFFFFF // 1
	StatusAvailable                                    = 0x2                // 2
	StatusBackingUp                                    = 0x4                // 4
	StatusConfiguringEnhancedMonitoring                = 0x8                // 8
	StatusConfiguringIAMDatabaseAuth                   = 0x10               // 16
	StatusConfiguringLogExports                        = 0x20               // 32
	StatusConvertingToVPC                              = 0x40               // 64
	StatusCreating                                     = 0x80               // 128
	StatusDeletePrecheck                               = 0x100              // 256
	StatusDeleting                                     = 0x200              // 512
	StatusFailed                                       = 0x400              // 1024
	StatusInaccessibleEncryptionCredentials            = 0x800              // 2048
	StatusInaccessibleEncryptionCredentialsRecoverable = 0x1000             // 4096
	StatusIncompatibleNetwork                          = 0x2000             // 8192
	StatusIncompatibleOptionGroup                      = 0x4000             // 16384
	StatusIncompatibleParameters                       = 0x8000             // 32768
	StatusIncompatibleRestore                          = 0x10000            // 65536
	StatusInsufficientCapacity                         = 0x20000            // 131072
	StatusMaintenance                                  = 0x40000            // 262144
	StatusModifying                                    = 0x80000            // 524288
	StatusMovingToVPC                                  = 0x100000           // 1048576
	StatusRebooting                                    = 0x200000           // 2097152
	StatusResettingMasterCredentials                   = 0x400000           // 4194304
	StatusRenaming                                     = 0x800000           // 8388608
	StatusRestoreError                                 = 0x1000000          // 16777216
	StatusStarting                                     = 0x2000000          // 33554432
	StatusStopped                                      = 0x4000000          // 67108864
	StatusStopping                                     = 0x8000000          // 134217728
	StatusStorageFull                                  = 0x10000000         // 268435456
	StatusStorageOptimization                          = 0x20000000         // 536870912
	StatusUpgrading                                    = 0x40000000         // 1073741824
)

func getWriterInstance(rdsClient *rds.RDS, rdsClusterName string) (*rds.DBInstance, error) {
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

func getReaderInstances(rdsClient *rds.RDS, rdsClusterName string, statusFilter uint64) ([]*rds.DBInstance, uint, error) {
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
		return nil, 0, fmt.Errorf("error describing RDS instances", err)
	}

	writerInstance, err := getWriterInstance(rdsClient, rdsClusterName)
	if err != nil {
		return nil, 0, fmt.Errorf("error describing RDS writer instance", err)
	}

	readerInstances := make([]*rds.DBInstance, 0)
	for _, instance := range describeOutput.DBInstances {
		if aws.StringValue(writerInstance.DBInstanceIdentifier) != aws.StringValue(instance.DBInstanceIdentifier) {
			instanceStatus := getStatusBitMask(*instance.DBInstanceStatus)
			if statusFilter == StatusAll || (instanceStatus&statusFilter != 0) {
				readerInstances = append(readerInstances, instance)
			}
		}
	}
	return readerInstances, uint(len(readerInstances)), nil
}

func waitForInstancesAvailable(rdsClient *rds.RDS, instanceIdentifiers []string) error {
	describeInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String("dummy"), // Placeholder value, it will be overridden in the loop
	}

	allInstancesReady := false
	for !allInstancesReady {
		allInstancesReady = true // Assume all instances are ready until proven otherwise

		for _, instanceIdentifier := range instanceIdentifiers {
			describeInput.DBInstanceIdentifier = aws.String(instanceIdentifier)

			describeOutput, err := rdsClient.DescribeDBInstances(describeInput)
			if err != nil {
				return fmt.Errorf("failed to describe RDS instance %s: %v", instanceIdentifier, err)
			}

			if len(describeOutput.DBInstances) == 0 {
				return fmt.Errorf("RDS instance %s not found", instanceIdentifier)
			}

			instanceStatus := *describeOutput.DBInstances[0].DBInstanceStatus
			if instanceStatus != "available" {
				fmt.Printf("Instance %s is not yet 'Available' (current status: %s)\n", instanceIdentifier, instanceStatus)
				allInstancesReady = false // At least one instance is not ready, so not all are ready
			}
		}

		if !allInstancesReady {
			time.Sleep(10 * time.Second)
		}
	}

	return nil
}

func getStatusBitMask(status string) uint64 {
	switch status {
	case "available":
		return StatusAvailable
	case "backing-up":
		return StatusBackingUp
	case "configuring-enhanced-monitoring":
		return StatusConfiguringEnhancedMonitoring
	case "configuring-iam-database-auth":
		return StatusConfiguringIAMDatabaseAuth
	case "configuring-log-exports":
		return StatusConfiguringLogExports
	case "converting-to-vpc":
		return StatusConvertingToVPC
	case "creating":
		return StatusCreating
	case "delete-precheck":
		return StatusDeletePrecheck
	case "deleting":
		return StatusDeleting
	case "failed":
		return StatusFailed
	case "inaccessible-encryption-credentials":
		return StatusInaccessibleEncryptionCredentials
	case "inaccessible-encryption-credentials-recoverable":
		return StatusInaccessibleEncryptionCredentialsRecoverable
	case "incompatible-network":
		return StatusIncompatibleNetwork
	case "incompatible-option-group":
		return StatusIncompatibleOptionGroup
	case "incompatible-parameters":
		return StatusIncompatibleParameters
	case "incompatible-restore":
		return StatusIncompatibleRestore
	case "insufficient-capacity":
		return StatusInsufficientCapacity
	case "maintenance":
		return StatusMaintenance
	case "modifying":
		return StatusModifying
	case "moving-to-vpc":
		return StatusMovingToVPC
	case "rebooting":
		return StatusRebooting
	case "resetting-master-credentials":
		return StatusResettingMasterCredentials
	case "renaming":
		return StatusRenaming
	case "restore-error":
		return StatusRestoreError
	case "starting":
		return StatusStarting
	case "stopped":
		return StatusStopped
	case "stopping":
		return StatusStopping
	case "storage-full":
		return StatusStorageFull
	case "storage-optimization":
		return StatusStorageOptimization
	case "upgrading":
		return StatusUpgrading
	default:
		return 0 // Return 0 for unknown status or status not specified in the constants
	}
}

func generateRandomUID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	uid := make([]byte, 8)
	for i := range uid {
		uid[i] = charset[rand.Intn(len(charset))]
	}
	return string(uid)
}
