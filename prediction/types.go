package prediction

import "predictive-rds-scaler/scaler"

type Driver interface {
	Initialize(config map[string]interface{}) error
	Store(status scaler.InstanceStatus) error
	Predict(inputData []byte) ([]byte, error)
}

type Service struct {
	driver Driver
}
