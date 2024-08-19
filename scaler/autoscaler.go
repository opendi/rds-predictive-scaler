package scaler

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/rs/zerolog"
	"math"
	"predictive-rds-scaler/metrics"
	"predictive-rds-scaler/types"
	"strconv"
	"time"
)

type Scaler struct {
	config       *types.Config
	scalerStatus types.Cooldown
	rdsClient    *rds.RDS
	logger       *zerolog.Logger
	broadcast    chan types.Broadcast
	metrics      *metrics.Metrics
}

func New(conf *types.Config, logger *zerolog.Logger, awsSession *session.Session, broadcast chan types.Broadcast) (*Scaler, error) {
	rdsClient := rds.New(awsSession, &aws.Config{
		Region: aws.String(conf.AwsRegion),
	})

	cloudwatchMetrics := metrics.New(*conf, logger, awsSession)

	return &Scaler{
		config:       conf,
		scalerStatus: types.Cooldown{Threshold: 0},
		rdsClient:    rdsClient,
		metrics:      cloudwatchMetrics,
		logger:       logger,
		broadcast:    broadcast,
	}, nil
}

func (s *Scaler) Run() {
	ticker := time.NewTicker(10 * time.Second)

	boostHours, err := parseBoostHours(s.config.BoostHours)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error parsing scale out hours")
	}

	for range ticker.C {
		s.scale(boostHours)
	}
}

func (s *Scaler) Shutdown() {
	close(s.broadcast)
}

func (s *Scaler) scale(boostHours []int) {
	// determine current status
	clusterStatus, err := s.getClusterStatus()
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting cluster status")
		return
	}

	s.logger.Info().
		Str("AverageCPUUtilization", strconv.FormatFloat(clusterStatus.AverageCPUUtilization, 'f', 2, 64)).
		Uint("CurrentActiveReaders", clusterStatus.CurrentActiveReaders).
		Uint("OptimalSize", clusterStatus.OptimalSize).
		Msg("Cluster status")

	// broadcast current status for UI
	s.submitBroadcast(&types.Broadcast{MessageType: "clusterStatus", Data: clusterStatus})

	// receive historical data
	historicStatus, err := s.metrics.GetHistoricClusterStatus(s.config.PlanAheadTime)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting historic cluster status")
		return
	}

	s.logger.Info().
		Str("AverageCPUUtilization", strconv.FormatFloat(historicStatus.AverageCPUUtilization, 'f', 2, 64)).
		Uint("CurrentActiveReaders", historicStatus.CurrentActiveReaders).
		Uint("OptimalSize", historicStatus.OptimalSize).
		Msg("Historic status")

	s.submitBroadcast(&types.Broadcast{MessageType: "clusterStatusPrediction", Data: historicStatus})

	minInstances := s.config.MinInstances
	if isBoostHour(time.Now().In(time.UTC).Hour(), boostHours) {
		minInstances = s.config.MinInstances + 1
	}
	maxOptimalSize := math.Max(float64(clusterStatus.OptimalSize), float64(historicStatus.OptimalSize))
	maxWithMinInstances := math.Max(float64(minInstances), maxOptimalSize)
	predictedOptimalSizeFloat := math.Min(float64(s.config.MaxInstances), maxWithMinInstances)
	predictedOptimalSize := uint(predictedOptimalSizeFloat)

	if predictedOptimalSize == clusterStatus.CurrentActiveReaders {
		s.logger.Info().
			Uint("Actual", clusterStatus.CurrentActiveReaders).
			Uint("Optimal", predictedOptimalSize).
			Msg("Cluster size is optimal")
	}

	if predictedOptimalSize > clusterStatus.CurrentActiveReaders {
		s.logger.Info().
			Uint("Actual", clusterStatus.CurrentActiveReaders).
			Uint("Optimal", predictedOptimalSize).
			Msg("Cluster size is below Optimal size, scaling out")

		if s.scalerStatus.IsScaling {
			s.logger.Info().Msg("Skipping scale out: Scaling operation already in progress")
			return
		}

		err := s.scaleOut(s.config.InstanceNamePrefix, predictedOptimalSize-clusterStatus.CurrentActiveReaders)
		if err != nil {
			return
		}
	}

	if predictedOptimalSize < clusterStatus.CurrentActiveReaders {
		s.logger.Info().
			Uint("Actual", clusterStatus.CurrentActiveReaders).
			Uint("Optimal", predictedOptimalSize).
			Msg("Cluster size is above Optimal size, scaling in")

		if s.scalerStatus.IsScaling {
			s.logger.Info().Msg("Skipping scale in: Scaling operation already in progress")
			return
		}

		err := s.scaleIn(clusterStatus.CurrentActiveReaders - predictedOptimalSize)
		if err != nil {
			s.logger.Error().Err(err).Msg("Error scaling in")
		}
	}
}

func (s *Scaler) GetClusterStatusHistory(duration time.Duration) []*types.ClusterStatus {
	start := time.Now().In(time.UTC).Add(-1 * duration)
	statusHistory, err := s.metrics.GetClusterStatus(start, time.Now().In(time.UTC), 5*time.Minute)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting cluster status history")
		return nil
	}
	return statusHistory
}

func (s *Scaler) GetClusterStatusPredictionHistory(duration time.Duration) []*types.ClusterStatus {
	end := time.Now().In(time.UTC).Add(-7 * 24 * time.Hour).Add(s.config.PlanAheadTime)
	start := end.Add(-1 * duration)
	statusPrediction, err := s.metrics.GetClusterStatus(start, end, 5*time.Minute)

	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting cluster status prediction history")
		return nil
	}

	for key, predictedStatus := range statusPrediction {
		statusPrediction[key].Timestamp = predictedStatus.Timestamp.
			Add(7 * 24 * time.Hour).         // Add 7 days to the timestamp to get the predicted time
			Add(-1 * s.config.PlanAheadTime) // Shift time back by the PlanAheadTime
	}
	return statusPrediction
}

func (s *Scaler) getClusterStatus() (*types.ClusterStatus, error) {
	var totalCPUUtilization float64
	var clusterStatus = types.ClusterStatus{
		Identifier: s.config.RdsClusterName,
		Timestamp:  time.Now().In(time.UTC),
	}

	//writerInstance, err := s.getWriterInstance()
	writerInstance, err := s.getWriterInstance()
	if err != nil {
		return nil, fmt.Errorf("didn't get writer instance: %v", err)
	}

	readerInstances, err := s.getReaderInstances(StatusAll)
	if err != nil {
		return nil, fmt.Errorf("didn't get reader instances: %v", err)
	}

	// Collect information about the writer readerInstance
	writerUtilization, err := s.metrics.GetCurrentInstanceUtilization(writerInstance)
	if err != nil {
		return nil, fmt.Errorf("didn't get current CPU utilization: %v", err)
	}

	writerStatus := types.InstanceStatus{
		Identifier:     *writerInstance.DBInstanceIdentifier,
		IsWriter:       true,
		Status:         *writerInstance.DBInstanceStatus,
		CPUUtilization: writerUtilization,
	}

	s.logInstanceStatus(writerStatus)

	totalCPUUtilization += writerStatus.CPUUtilization
	clusterStatus.CurrentActiveReaders = 1
	clusterStatus.Instances = append(clusterStatus.Instances, writerStatus)

	// Collect information about reader instances
	for _, readerInstance := range readerInstances {
		readerUtilization, err := s.metrics.GetCurrentInstanceUtilization(readerInstance)
		if err != nil {
			return nil, err
		}

		readerStatus := types.InstanceStatus{
			Identifier:     *readerInstance.DBInstanceIdentifier,
			IsWriter:       false,
			Status:         *readerInstance.DBInstanceStatus,
			CPUUtilization: readerUtilization,
		}

		s.logInstanceStatus(readerStatus)

		if readerStatus.Status == "available" {
			clusterStatus.CurrentActiveReaders++
			totalCPUUtilization += readerStatus.CPUUtilization
		}
		clusterStatus.Instances = append(clusterStatus.Instances, readerStatus)
	}

	clusterStatus.AverageCPUUtilization = totalCPUUtilization / float64(clusterStatus.CurrentActiveReaders)
	clusterStatus.OptimalSize = s.metrics.CalculateOptimalClusterSize(clusterStatus.AverageCPUUtilization, clusterStatus.CurrentActiveReaders, s.config.MinInstances)

	return &clusterStatus, nil
}

func (s *Scaler) logInstanceStatus(writerStatus types.InstanceStatus) {
	s.logger.Info().
		Str("Identifier", writerStatus.Identifier).
		Bool("IsWriter", writerStatus.IsWriter).
		Str("Status", writerStatus.Status).
		Float64("CPUUtilization", writerStatus.CPUUtilization).
		Msg("Instance status")
}

func (s *Scaler) logScalerStatus(cpuUtilization float64, currentSize int) {
	s.logger.Info().
		Str("CPUUtilization", strconv.FormatFloat(cpuUtilization, 'f', 2, 64)).
		Int("CurrentReaders", currentSize).
		Float64("PlanAheadTime", s.config.PlanAheadTime.Seconds()).
		Msg("Scaler status")
}

func (s *Scaler) submitBroadcast(broadcast *types.Broadcast) {
	if broadcast.Data != nil {
		go func() {
			s.broadcast <- *broadcast
		}()
	}
}

func (s *Scaler) scaleOut(readerNamePrefix string, numInstances uint) error {
	s.scalerStatus.IsScaling = true

	currentHour := time.Now().In(time.UTC).Hour()
	newReaderInstanceNames := make([]string, numInstances)

	for i := 0; i < int(numInstances); i++ {
		// Get the current writer instance
		writerInstance, err := s.getWriterInstance()
		if err != nil {
			return fmt.Errorf("failed to get current writer instance: %v", err)
		}

		readerInstances, err := s.getReaderInstances(StatusAll ^ StatusDeleting)
		if (len(readerInstances) + 1) >= int(s.config.MaxInstances) {
			return fmt.Errorf("max number of instances reached")
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
		start := time.Now().In(time.UTC)
		err := s.waitForInstancesAvailable(newReaderInstanceNames)
		elapsed := time.Since(start)

		s.scalerStatus.LastScale = time.Now().In(time.UTC)
		s.scalerStatus.IsScaling = false

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
	s.scalerStatus.IsScaling = true

	readerInstances, err := s.getReaderInstances(StatusAll)

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

		go func() {
			err := s.waitUntilInstanceIsDeleted(*instance.DBInstanceIdentifier)
			s.scalerStatus.IsScaling = false
			if err != nil {
				return
			}
		}()

		s.logger.Info().Str("InstanceIdentifier", *instance.DBInstanceIdentifier).Str("InstanceStatus", *instance.DBInstanceStatus).Msg("Instance is deleting")
	}

	return nil
}

func (s *Scaler) Stop() {
	s.logger.Info().Msg("Stopping scaler")
	close(s.broadcast)
}
