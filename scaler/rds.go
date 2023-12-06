package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/rds"
	"math/rand"
	"strconv"
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

func (s *Scaler) getWriterInstance() (*rds.DBInstance, error) {
	describeInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(s.config.RdsClusterName),
	}

	clusterOutput, err := s.rdsClient.DescribeDBClusters(describeInput)
	if err != nil {
		return nil, err
	}

	if len(clusterOutput.DBClusters) == 0 {
		return nil, fmt.Errorf("aurora cluster not found: %s", s.config.RdsClusterName)
	}

	// Loop through the cluster members to find the writer instance
	for _, member := range clusterOutput.DBClusters[0].DBClusterMembers {
		if aws.BoolValue(member.IsClusterWriter) {
			describeInstanceInput := &rds.DescribeDBInstancesInput{
				DBInstanceIdentifier: member.DBInstanceIdentifier,
			}

			instanceOutput, err := s.rdsClient.DescribeDBInstances(describeInstanceInput)
			if err != nil {
				return nil, err
			}

			if len(instanceOutput.DBInstances) > 0 {
				return instanceOutput.DBInstances[0], nil
			}

			return nil, fmt.Errorf("writer instance not found in cluster: %s", s.config.RdsClusterName)
		}
	}

	return nil, fmt.Errorf("writer instance not found in cluster: %s", s.config.RdsClusterName)
}

func (s *Scaler) createReaderInstance(readerName string, writerInstance *rds.DBInstance) (*rds.CreateDBInstanceOutput, error) {
	// Use the writer instance's configuration as a template for the new reader instance
	readerDBInstance := &rds.CreateDBInstanceInput{
		DBInstanceClass:         writerInstance.DBInstanceClass,
		Engine:                  writerInstance.Engine,
		DBClusterIdentifier:     aws.String(s.config.RdsClusterName),
		DBInstanceIdentifier:    aws.String(readerName),
		PubliclyAccessible:      aws.Bool(false),
		MultiAZ:                 writerInstance.MultiAZ,
		CopyTagsToSnapshot:      writerInstance.CopyTagsToSnapshot,
		AutoMinorVersionUpgrade: writerInstance.AutoMinorVersionUpgrade,
		DBParameterGroupName:    writerInstance.DBParameterGroups[0].DBParameterGroupName,
		CACertificateIdentifier: writerInstance.CACertificateIdentifier,
	}

	// Perform the scaling operation to add a reader to the cluster
	return s.rdsClient.CreateDBInstance(readerDBInstance)
}

func (s *Scaler) getReaderInstances(statusFilter uint64) ([]*rds.DBInstance, uint, error) {
	describeInput := &rds.DescribeDBInstancesInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-cluster-id"),
				Values: []*string{aws.String(s.config.RdsClusterName)},
			},
		},
	}

	describeOutput, err := s.rdsClient.DescribeDBInstances(describeInput)
	if err != nil {
		return nil, 0, fmt.Errorf("error describing RDS instances: %v", err)
	}

	writerInstance, err := s.getWriterInstance()
	if err != nil {
		return nil, 0, fmt.Errorf("error describing RDS writer instance: %v", err)
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
	// reader instances, counting writer in
	return readerInstances, uint(len(readerInstances)) + 1, nil
}

func (s *Scaler) waitForInstancesAvailable(instanceIdentifiers []string) error {
	describeInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String("dummy"), // Placeholder value, it will be overridden in the loop
	}

	allInstancesReady := false
	for !allInstancesReady {
		allInstancesReady = true // Assume all instances are ready until proven otherwise

		for _, instanceIdentifier := range instanceIdentifiers {
			describeInput.DBInstanceIdentifier = aws.String(instanceIdentifier)

			describeOutput, err := s.rdsClient.DescribeDBInstances(describeInput)
			if err != nil {
				return fmt.Errorf("failed to describe RDS instance %s: %v", instanceIdentifier, err)
			}

			if len(describeOutput.DBInstances) == 0 {
				return fmt.Errorf("RDS instance %s not found", instanceIdentifier)
			}

			instanceStatus := *describeOutput.DBInstances[0].DBInstanceStatus
			if instanceStatus != "available" {
				s.logger.Info().Str("InstanceIdentifier", instanceIdentifier).Str("InstanceStatus", instanceStatus).Msg("Instance is not yet 'Available'")
				allInstancesReady = false // At least one instance is not ready, so not all are ready
			}
		}

		if !allInstancesReady {
			time.Sleep(10 * time.Second)
		}
	}

	return nil
}

func (s *Scaler) waitUntilInstanceDeletable(instanceIdentifier string) error {
	describeInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(instanceIdentifier),
	}

	for {
		describeOutput, err := s.rdsClient.DescribeDBInstances(describeInput)
		if err != nil {
			return fmt.Errorf("failed to describe RDS instance %s: %v", instanceIdentifier, err)
		}

		if len(describeOutput.DBInstances) == 0 {
			return fmt.Errorf("RDS instance %s not found", instanceIdentifier)
		}

		instanceStatus := *describeOutput.DBInstances[0].DBInstanceStatus
		if isDeletableStatus(instanceStatus) {
			return nil
		}

		s.logger.Info().Str("InstanceIdentifier", instanceIdentifier).Str("InstanceStatus", instanceStatus).Msg("Waiting for instance to become deletable")
		time.Sleep(5 * time.Second)
	}
}

func (s *Scaler) waitUntilInstanceIsDeleted(instanceIdentifier string) error {
	describeInput := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(instanceIdentifier),
	}

	for {
		describeOutput, err := s.rdsClient.DescribeDBInstances(describeInput)
		if err != nil {
			return fmt.Errorf("failed to describe RDS instance %s: %v", instanceIdentifier, err)
		}

		if len(describeOutput.DBInstances) == 0 {
			return nil
		}

		instanceStatus := *describeOutput.DBInstances[0].DBInstanceStatus

		s.logger.Info().Str("InstanceIdentifier", instanceIdentifier).Str("InstanceStatus", instanceStatus).Msg("Waiting for instance to be deleted")
		time.Sleep(5 * time.Second)
	}
}

func (s *Scaler) saveCooldownStatus(tagKey string, lastTime time.Time) error {
	clusterArn, err := s.getClusterArn()
	if err != nil {
		return err
	}

	tagInput := &rds.AddTagsToResourceInput{
		ResourceName: aws.String(clusterArn), // Assuming you have the RDS cluster ARN available in your Scaler struct
		Tags: []*rds.Tag{
			{
				Key:   aws.String(tagKey),
				Value: aws.String(strconv.FormatInt(lastTime.Unix(), 10)), // Store as Unix timestamp
			},
		},
	}

	_, err = s.rdsClient.AddTagsToResource(tagInput)
	return err
}

func (s *Scaler) getClusterArn() (string, error) {
	describeDBClustersInput := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(s.config.RdsClusterName),
	}
	describeDBClustersOutput, err := s.rdsClient.DescribeDBClusters(describeDBClustersInput)
	if err != nil {
		return "", fmt.Errorf("failed to describe DB clusters: %v", err)
	}

	return *describeDBClustersOutput.DBClusters[0].DBClusterArn, nil
}

func (s *Scaler) getClusterTags(clusterArn string) (map[string]string, error) {
	input := &rds.ListTagsForResourceInput{
		ResourceName: aws.String(clusterArn),
	}

	result, err := s.rdsClient.ListTagsForResource(input)
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string)
	for _, tag := range result.TagList {
		tags[*tag.Key] = *tag.Value
	}

	return tags, nil
}

func isDeletableStatus(status string) bool {
	invalidStatus := []string{"deleting", "modifying", "maintenance", "rebooting"}
	return !containsString(invalidStatus, status)
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
