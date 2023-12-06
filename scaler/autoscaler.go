package scaler

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/rs/zerolog"
	"predictive-rds-scaler/history"
	"strconv"
	"time"
)

const readerNamePrefix = "predictive-autoscaling-"

func New(config Config, logger *zerolog.Logger, awsSession *session.Session, broadcast chan Broadcast) (*Scaler, error) {
	rdsClient := rds.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	cloudWatchClient := cloudwatch.New(awsSession, &aws.Config{
		Region: aws.String(config.AwsRegion),
	})

	ctx := context.Background()
	dynamoDbHistory, err := history.New(ctx, logger, awsSession, config.AwsRegion, config.RdsClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create DynamoDB history: %v", err)
	}

	return &Scaler{
		config:           config,
		scaleOutStatus:   Cooldown{threshold: 0},
		scaleInStatus:    Cooldown{threshold: 0},
		rdsClient:        rdsClient,
		cloudWatchClient: cloudWatchClient,
		dynamoDbHistory:  dynamoDbHistory,
		logger:           logger,
		broadcast:        broadcast,
	}, nil
}

func (s *Scaler) Run() {
	ticker := time.NewTicker(10 * time.Second)

	boostHours, err := parseBoostHours(s.config.BoostHours)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error parsing scale out hours")
	}

	for range ticker.C {
		s.processScaling(boostHours)
	}
}

func (s *Scaler) Shutdown() {
	close(s.broadcast)
}

func (s *Scaler) processScaling(boostHours []int) {
	writerInstance, err := s.getWriterInstance()
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting writer instance")
		return
	}

	readerInstances, currentSize, err := s.getReaderInstances(StatusAll ^ StatusDeleting)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting reader instances")
		return
	}

	clusterStatus := make([]InstanceStatus, 0)

	// Collect information about reader instances
	for _, instance := range readerInstances {
		instanceInfo := InstanceStatus{
			Name:           *instance.DBInstanceIdentifier,
			IsWriter:       false,
			Status:         *instance.DBInstanceStatus,
			CPUUtilization: s.getInstanceUtilization(instance),
		}
		clusterStatus = append(clusterStatus, instanceInfo)
	}

	// Collect information about the writer instance
	writerStatus := InstanceStatus{
		Name:           *writerInstance.DBInstanceIdentifier,
		IsWriter:       true,
		Status:         *writerInstance.DBInstanceStatus,
		CPUUtilization: s.getInstanceUtilization(writerInstance),
	}
	clusterStatus = append(clusterStatus, writerStatus)

	prediction, err := s.getUtilizationPrediction(clusterStatus)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting CPU utilization")
		return
	}

	minInstances := s.config.MinInstances
	if isBoostHour(time.Now().Hour(), boostHours) {
		minInstances = s.config.MinInstances + s.config.ScaleOutStep
	}

	s.submitBroadcast(&Broadcast{"clusterStatus", clusterStatus})

	desiredClusterSize := s.calculateDesiredClusterSize(prediction.MaxCpuUtilization, prediction.NumReaders, minInstances)

	if desiredClusterSize == currentSize {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", desiredClusterSize).
			Msg("Cluster size is optimal")
	}

	if desiredClusterSize > currentSize {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", desiredClusterSize).
			Msg("Cluster size is below desired size, scaling out")

		if s.scaleOutStatus.IsScaling {
			s.logger.Info().Msg("Skipping scale out: Scaling operation already in progress")
			return
		}

		err := s.scaleOut(readerNamePrefix, desiredClusterSize-currentSize)
		if err != nil {
			return
		}
	}

	if desiredClusterSize < currentSize {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", desiredClusterSize).
			Msg("Cluster size is above desired size, scaling in")

		if s.scaleInStatus.IsScaling {
			s.logger.Info().Msg("Skipping scale in: Scaling operation already in progress")
			return
		}

		err := s.scaleIn(currentSize - desiredClusterSize)
		if err != nil {
			s.logger.Error().Err(err).Msg("Error scaling in")
		}
	}
}

func (s *Scaler) logScalerStatus(cpuUtilization float64, currentSize int) {
	s.logger.Info().
		Str("CPUUtilization", strconv.FormatFloat(cpuUtilization, 'f', 2, 64)).
		Int("CurrentReaders", currentSize).
		Float64("PlanAheadTime", s.config.PlanAheadTime.Seconds()).
		Msg("Scaler status")
}

func (s *Scaler) getUtilizationPrediction(instanceStatus []InstanceStatus) (*history.UtilizationSnapshot, error) {
	historicQueryTime := time.Now().
		Add(-time.Hour * 24 * 7).
		Truncate(10 * time.Second)

	prediction, err := s.dynamoDbHistory.GetValue(historicQueryTime, s.config.PlanAheadTime)
	if prediction != nil {
		s.submitBroadcast(&Broadcast{"prediction", prediction})
	} else {
		s.logger.Warn().Msg("No historic value found")
	}

	currentCpuUtilization, currentActiveReaderCount, err := s.getMaxCPUUtilization(instanceStatus)
	if err != nil {
		return nil, fmt.Errorf("error getting max CPU utilization: %v", err)
	}

	snapshot := history.UtilizationSnapshot{
		Timestamp:         time.Now().Truncate(10 * time.Second),
		ClusterName:       s.config.RdsClusterName,
		NumReaders:        currentActiveReaderCount,
		MaxCpuUtilization: currentCpuUtilization,
		PredictedValue:    false,
		TTL:               time.Now().Add(8 * 24 * time.Hour).Unix(),
	}
	s.submitBroadcast(&Broadcast{"snapshot", snapshot})

	// Save the item to DynamoDB when scaling is required
	if err = s.dynamoDbHistory.SaveItem(&snapshot); err != nil {
		s.logger.Error().Err(err).Msg("Error saving item to DynamoDB")
	}

	s.logScalerStatus(prediction.MaxCpuUtilization, len(instanceStatus))

	// If a historic value was found, use it to predict the current CPU utilization
	if prediction != nil && prediction.MaxCpuUtilization > 0 {
		// how high would the historic CPU utilization be with the current reader count?
		s.logger.Info().
			Float64("Historic 98 percentile", prediction.MaxCpuUtilization).
			Float64("CurrentCPUUtilization", currentCpuUtilization).
			Msg("Historic value found.")

		return prediction, nil
	}
	return &snapshot, nil
}

func (s *Scaler) submitBroadcast(broadcast *Broadcast) {
	if broadcast.Data != nil {
		go func() {
			s.broadcast <- *broadcast
		}()
	}
}

func (s *Scaler) scaleOut(readerNamePrefix string, numInstances uint) error {
	s.scaleOutStatus.IsScaling = true
	defer func() {
		s.scaleOutStatus.IsScaling = false
	}()

	currentHour := time.Now().Hour()
	newReaderInstanceNames := make([]string, numInstances)

	for i := 0; i < int(numInstances); i++ {
		// Get the current writer instance
		writerInstance, err := s.getWriterInstance()
		if err != nil {
			return fmt.Errorf("failed to get current writer instance: %v", err)
		}

		// Generate a random UID for the new reader instance name
		randomUID := generateRandomUID()

		// Create the reader instance name with the prefix, current scale-out hour, and random UID
		readerName := fmt.Sprintf("%s%d-%s", readerNamePrefix, currentHour, randomUID)

		_, err = s.createReaderInstance(readerName, writerInstance)
		if err != nil {
			return fmt.Errorf("failed to add reader instance: %v", err)
		}

		s.logger.Info().Str("NewReaderInstanceName", readerName).Msg("Scaling out operation successful")

		// Add the new reader instance name to the slice
		newReaderInstanceNames[i] = readerName
	}

	go func() {
		start := time.Now()
		err := s.waitForInstancesAvailable(newReaderInstanceNames)
		elapsed := time.Since(start)

		s.scaleOutStatus.LastScale = time.Now()
		if err != nil {
			s.logger.Error().Err(err).Msg("Error setting cooldown status")
		}

		if err != nil {
			s.logger.Error().Err(err).Msg("Error waiting for instances to become 'Available'")
			return
		}

		// Adjust PlanAheadTime if elapsed time + buffer is greater
		if adjustedTime := elapsed + 60*time.Second; adjustedTime > s.config.PlanAheadTime {
			s.config.PlanAheadTime = adjustedTime
			s.logger.Info().Dur("AdjustedPlanAheadTime", s.config.PlanAheadTime).Msg("PlanAheadTime adjusted")
		}
	}()

	return nil
}

func (s *Scaler) scaleIn(numInstances uint) error {
	s.scaleInStatus.IsScaling = true

	defer func() {
		// Reset the isScaling flag
		s.scaleInStatus.IsScaling = false
	}()

	readerInstances, _, err := s.getReaderInstances(StatusAll)

	if err != nil {
		return fmt.Errorf("failed to get reader instances: %v", err)
	}

	for i := 0; i < int(numInstances); i++ {
		// Check if there are any reader instances available to scale in
		if len(readerInstances) == 0 {
			break
		}

		// Choose a reader instance to remove
		instance := readerInstances[i]

		// Check if the instance is in the process of deletion, and it's the last remaining reader instance
		if *instance.DBInstanceStatus == "deleting" && len(readerInstances) == 1 {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("The last remaining instance is already in status 'deleting'. Will not remove it to avoid service disruption.")
			break
		}

		// Skip over instances with the status "deleting"
		if *instance.DBInstanceStatus == "deleting" {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("Skipping instance already in status 'deleting'")
			err := s.waitUntilInstanceIsDeleted(*instance.DBInstanceIdentifier)
			if err != nil {
				return err
			}
			continue
		}

		// Wait for the instance to become deletable
		err := s.waitUntilInstanceDeletable(*instance.DBInstanceIdentifier)
		if err != nil {
			return fmt.Errorf("failed to wait for instance to become deletable: %v", err)
		}

		// Remove the reader instance
		_, err = s.rdsClient.DeleteDBInstance(&rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: instance.DBInstanceIdentifier,
			SkipFinalSnapshot:    aws.Bool(true),
		})
		if err != nil {
			return fmt.Errorf("failed to remove reader instance: %v", err)
		}

		err = s.waitUntilInstanceIsDeleted(*instance.DBInstanceIdentifier)
		if err != nil {
			return err
		}
		s.logger.Info().Str("InstanceIdentifier", *instance.DBInstanceIdentifier).Str("InstanceStatus", *instance.DBInstanceStatus).Msg("Instance is deleting")
	}

	return nil
}
