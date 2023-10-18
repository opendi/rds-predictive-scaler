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

	err = s.initCooldownStatus()
	if err != nil {
		s.logger.Error().Err(err).Msg("Error initializing cooldown status")
	}

	for range ticker.C {
		s.processScaling(boostHours)
	}
}

func (s *Scaler) Shutdown() {
	close(s.broadcast)
}

func (s *Scaler) initCooldownStatus() error {
	err := s.loadAndSetCooldownStatus("ScaleOutStatusTimeout", &s.scaleOutStatus)
	if err != nil {
		return err
	}

	err = s.loadAndSetCooldownStatus("ScaleInStatusTimeout", &s.scaleInStatus)
	if err != nil {
		return err
	}

	return nil
}

func (s *Scaler) loadAndSetCooldownStatus(tagName string, cooldownStatus *Cooldown) error {
	status, err := s.loadCooldownStatus(tagName)
	if err != nil {
		return err
	}

	*cooldownStatus = status
	return nil
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
			CPUUtilization: s.getInstanceUtilization(instance), // Use your existing function to get CPU utilization
		}
		clusterStatus = append(clusterStatus, instanceInfo)
	}

	// Collect information about the writer instance
	writerStatus := InstanceStatus{
		Name:           *writerInstance.DBInstanceIdentifier,
		IsWriter:       true,
		Status:         *writerInstance.DBInstanceStatus,
		CPUUtilization: s.getInstanceUtilization(writerInstance), // Use your existing function to get CPU utilization
	}
	clusterStatus = append(clusterStatus, writerStatus)

	cpuUtilization, err := s.getUtilizationPrediction(clusterStatus)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error getting CPU utilization")
		return
	}

	minInstances := s.config.MinInstances
	if isBoostHour(time.Now().Hour(), boostHours) {
		minInstances = s.config.MinInstances + s.config.ScaleOutStep
	}

	s.logScalerStatus(cpuUtilization, currentSize)
	s.submitBroadcast(&Broadcast{"clusterStatus", clusterStatus})

	if s.inCooldown(s.scaleInStatus.Timeout) {
		s.submitBroadcast(&Broadcast{"scaleInStatus", s.scaleInStatus})
	}

	if s.inCooldown(s.scaleOutStatus.Timeout) {
		s.submitBroadcast(&Broadcast{"scaleOutStatus", s.scaleOutStatus})
	}

	if s.shouldScaleOut(cpuUtilization, currentSize, minInstances) {
		s.handleScaleOut(cpuUtilization, currentSize, minInstances)
	}

	if s.shouldScaleIn(cpuUtilization, currentSize, minInstances) {
		s.handleScaleIn(cpuUtilization, currentSize, minInstances)
	}
}

func (s *Scaler) logScalerStatus(cpuUtilization float64, currentSize uint) {
	s.logger.Info().
		Str("CPUUtilization", strconv.FormatFloat(cpuUtilization, 'f', 2, 64)).
		Uint("CurrentReaders", currentSize).
		Int("ScaleInCooldown", remainingCooldown(s.scaleInStatus.Timeout)).
		Int("ScaleOutCooldown", remainingCooldown(s.scaleOutStatus.Timeout)).
		Float64("PlanAheadTime", s.config.PlanAheadTime.Seconds()).
		Msg("Scaler status")
}

func (s *Scaler) handleScaleOut(cpuUtilization float64, currentSize, minInstances uint) {
	if currentSize < minInstances {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", minInstances).
			Msg("Should scale out, currently below minimum instances")
		return
	}

	if cpuUtilization > s.config.TargetCpuUtil && currentSize < s.config.MaxInstances {
		if s.scaleOutStatus.threshold < 3 {
			s.logger.Info().Msg("Skipping scale out, threshold not reached")
			s.scaleOutStatus.threshold++
			return
		}
		s.scaleOutStatus.threshold = 0

		scaleOutInstances := s.calculateScaleOutReaderCount(currentSize)
		if scaleOutInstances > 0 {
			s.logger.Info().Uint("ScaleOutInstances", scaleOutInstances).Msg("Scaling out instances")
			err := s.scaleOut(readerNamePrefix, scaleOutInstances)
			if err != nil {
				s.logger.Error().Err(err).Msg("Error scaling out")
			} else {
				s.scaleOutStatus.LastScale = time.Now()
				err = s.setCooldownStatus(30*time.Second, s.config.ScaleOutCooldown)
				if err != nil {
					s.logger.Error().Err(err).Msg("Error setting cooldown status")
				}
			}
		} else {
			s.logger.Info().Msg("Max instances reached. Cannot scale out.")
		}
	} else {
		s.scaleOutStatus.threshold = 0
	}
}

func (s *Scaler) handleScaleIn(cpuUtilization float64, currentSize, minInstances uint) {
	if !s.inCooldown(s.scaleInStatus.Timeout) && s.shouldScaleIn(cpuUtilization, currentSize, minInstances) {
		if s.scaleInStatus.threshold < 3 {
			s.logger.Info().Msg("Skipping scale in, threshold not reached")
			s.scaleInStatus.threshold++
			return
		}
		s.scaleInStatus.threshold = 0

		scaleInInstances := s.calculateScaleInReaderCount(currentSize, minInstances)
		if scaleInInstances > 0 {
			s.logger.Info().Uint("ScaleInInstances", scaleInInstances).Msg("Scaling in instances")
			err := s.scaleIn(scaleInInstances)
			if err != nil {
				s.logger.Error().Err(err).Msg("Error scaling in")
			} else {
				s.scaleInStatus.LastScale = time.Now()
				err = s.setCooldownStatus(s.config.ScaleInCooldown, 60*time.Second)
				if err != nil {
					s.logger.Error().Err(err).Msg("Error setting cooldown status")
				}
			}
		} else {
			s.logger.Info().Msg("Min instances reached. Cannot scale in.")
		}
	} else {
		s.scaleInStatus.threshold = 0
	}
}

func remainingCooldown(timeout time.Time) int {
	remaining := timeout.Sub(time.Now())
	if remaining < 0 {
		return 0
	}
	return int(remaining.Seconds())
}

func (s *Scaler) getUtilizationPrediction(instanceStatus []InstanceStatus) (float64, error) {
	historicQueryTime := time.Now().
		Add(-time.Hour * 24 * 7).
		Add(s.config.PlanAheadTime).
		Truncate(10 * time.Second)

	prediction, err := s.dynamoDbHistory.GetValue(historicQueryTime)
	if err != nil {
		return 0, fmt.Errorf("error getting historic value: %v", err)
	}
	if prediction != nil {
		s.submitBroadcast(&Broadcast{"prediction", prediction})
	} else {
		s.logger.Warn().Msg("No historic value found")
	}

	currentCpuUtilization, currentActiveReaderCount, err := s.getMaxCPUUtilization(instanceStatus)
	if err != nil {
		return 0, fmt.Errorf("error getting max CPU utilization: %v", err)
	}

	// If a historic value was found, use it to predict the current CPU utilization
	predictedValue := false
	if prediction != nil && prediction.MaxCpuUtilization > 0 {
		// how high would the historic CPU utilization be with the current reader count?
		predictedCpuUtilization := prediction.MaxCpuUtilization * (float64(prediction.NumReaders+1) / float64(currentActiveReaderCount+1))
		s.logger.Info().
			Float64("HistoricCpuUtilization", prediction.MaxCpuUtilization).
			Uint("HistoricReaderCount", prediction.NumReaders).
			Float64("PredictedCpuUtilization", predictedCpuUtilization).
			Float64("CurrentCPUUtilization", currentCpuUtilization).
			Msg("Historic value found.")

		if predictedCpuUtilization > currentCpuUtilization {
			currentCpuUtilization = predictedCpuUtilization
		}
		predictedValue = true
	}

	snapshot := history.UtilizationSnapshot{
		Timestamp:         time.Now().Truncate(10 * time.Second),
		ClusterName:       s.config.RdsClusterName,
		NumReaders:        currentActiveReaderCount,
		MaxCpuUtilization: currentCpuUtilization,
		PredictedValue:    predictedValue,
		TTL:               time.Now().Add(8 * 24 * time.Hour).Unix(),
	}

	// Save the item to DynamoDB when scaling is required
	if err = s.dynamoDbHistory.SaveItem(&snapshot); err != nil {
		return 0, fmt.Errorf("error saving item to DynamoDB: %v", err)
	}

	s.submitBroadcast(&Broadcast{"snapshot", snapshot})

	return currentCpuUtilization, nil
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

	startingInstances, numStartingInstances, err := s.getReaderInstances(StatusCreating | StatusConfiguringEnhancedMonitoring)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Error getting starting instances")
	}

	if numStartingInstances > 0 {
		go func() {
			s.logger.Info().Msg("Waiting for starting instances to be ready")
			instanceIdentifiers := make([]string, len(startingInstances))
			for i, instance := range startingInstances {
				instanceIdentifiers[i] = *instance.DBInstanceIdentifier
			}

			err = s.waitForInstancesAvailable(instanceIdentifiers)
			if err != nil {
				s.logger.Warn().Err(err).Msg("error waiting for starting instances to be ready")
			}

		}()
	}

	numInstances -= numStartingInstances
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
		instance := readerInstances[0]

		// Check if the instance is in the process of deletion, and it's the last remaining reader instance
		if *instance.DBInstanceStatus == "deleting" && len(readerInstances) == 1 {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("The last remaining instance is already in status 'deleting'. Will not remove it to avoid service disruption.")
			break
		}

		// Skip over instances with the status "deleting"
		if *instance.DBInstanceStatus == "deleting" {
			s.logger.Info().Str("InstanceID", *instance.DBInstanceIdentifier).Msg("Skipping instance already in status 'deleting'")
			numInstances++
			readerInstances = readerInstances[1:]
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
		s.logger.Info().Str("InstanceIdentifier", *instance.DBInstanceIdentifier).Str("InstanceStatus", *instance.DBInstanceStatus).Msg("Instance is deleting")

		// Remove the scaled-in instance from the list
		readerInstances = readerInstances[1:]
	}

	return nil
}

// shouldScaleOut returns true if scaling out is needed based on the current CPU utilization and the maximum number of instances.
func (s *Scaler) shouldScaleOut(cpuUtilization float64, currentSize, minInstances uint) bool {
	if s.scaleOutStatus.IsScaling {
		s.logger.Info().Msg("Skipping scale out: Scaling operation already in progress")
		return false
	}

	if s.inCooldown(s.scaleOutStatus.Timeout) {
		s.logger.Info().Msg("Skipping scale out: Scale out in cooldown")
		return false
	}

	if currentSize < minInstances {
		s.logger.Info().
			Uint("Actual", currentSize).
			Uint("Desired", minInstances).
			Msg("Should scale out, currently below minimum instances")
		return true
	}

	if cpuUtilization > s.config.TargetCpuUtil && currentSize < s.config.MaxInstances {
		s.logger.Info().
			Float64("CPUUtilization", cpuUtilization).
			Uint("CurrentSize", currentSize).
			Uint("MaxInstances", s.config.MaxInstances).
			Msg("Should scale out, currently above target CPU utilization")
		return true
	}

	s.logger.Info().Msg("No need to scale out")
	return false
}

// calculateScaleOutReaderCount calculates the number of instances to scale out based on the maximum number of instances and the current size.
func (s *Scaler) calculateScaleOutReaderCount(currentSize uint) uint {
	return minInt(s.config.ScaleOutStep, s.config.MaxInstances-currentSize)
}

// shouldScaleIn returns true if scaling in is needed based on the current CPU utilization and the minimum number of instances.
func (s *Scaler) shouldScaleIn(currentCpuUtilization float64, currentSize, minInstances uint) bool {
	if s.scaleInStatus.IsScaling {
		s.logger.Info().Msg("Skipping scale in: Scaling operation already in progress")
		return false
	}

	if s.inCooldown(s.scaleInStatus.Timeout) {
		s.logger.Info().Msg("Skipping scale in: Scale in in cooldown")
		return false
	}

	if currentCpuUtilization > s.config.TargetCpuUtil {
		return false
	}

	if currentSize < minInstances+s.config.ScaleInStep {
		s.logger.Info().Msg("Skipping scale in: Minimum instance threshold reached.")
		return false
	}

	if (currentSize - s.config.ScaleInStep) == 0 {
		if currentCpuUtilization <= s.config.TargetCpuUtil/2 {
			s.logger.Info().
				Float64("Threshold", s.config.TargetCpuUtil/2).
				Msg("Should scale in, CPU utilization is below threshold, scaling in to 0 reader instances.")
			return true
		} else {
			return false
		}
	}

	predictedCpuUtilization := (currentCpuUtilization * float64(currentSize)) / float64(currentSize-s.config.ScaleInStep)
	if predictedCpuUtilization <= s.config.TargetCpuUtil {
		s.logger.Info().
			Float64("PredictedCPUUtilization", predictedCpuUtilization).
			Msg("Should scale in, predicted CPU utilization is below target.")
		return true
	}

	s.logger.Info().Msg("No need to scale in.")
	return false
}

// calculateScaleInReaderCount calculates the number of instances to scale in based on the current size and the minimum number of instances.
func (s *Scaler) calculateScaleInReaderCount(currentSize, minInstances uint) uint {
	return minInt(s.config.ScaleInStep, currentSize-minInstances)
}

func (s *Scaler) loadCooldownStatus(tagName string) (Cooldown, error) {
	cooldown := Cooldown{}

	clusterArn, err := s.getClusterArn()
	if err != nil {
		return cooldown, err
	}

	tags, err := s.getClusterTags(clusterArn)
	if err != nil {
		return cooldown, err
	}

	lastTimeStr, ok := tags[tagName]
	if !ok {
		return cooldown, nil // Tag not found, return empty cooldown
	}

	unixTimestamp, err := strconv.ParseInt(lastTimeStr, 10, 64)
	if err != nil {
		return cooldown, fmt.Errorf("failed to parse LastTime from tag: %v", err)
	}

	lastTime := time.Unix(unixTimestamp, 0)

	cooldown.Timeout = lastTime
	return cooldown, nil
}

func (s *Scaler) setCooldownStatus(scaleInCooldown time.Duration, scaleOutCooldown time.Duration) error {
	var err error = nil
	s.scaleInStatus.Timeout = time.Now().Add(scaleInCooldown)
	err = s.saveCooldownStatus("ScaleInStatusTimeout", s.scaleInStatus.Timeout)

	s.scaleOutStatus.Timeout = time.Now().Add(scaleOutCooldown)
	err = s.saveCooldownStatus("ScaleOutStatusTimeout", s.scaleOutStatus.Timeout)

	return err
}

func (s *Scaler) inCooldown(timeout time.Time) bool {
	if time.Now().Before(timeout) {
		return true
	}
	return false
}
