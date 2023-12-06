package prediction

import "errors"

func CreateDriver(driverName string, config map[string]interface{}) (Driver, error) {
	switch driverName {
	case "googleBigData":
		return NewGoogleBigDataDriver(), nil
	default:
		return nil, errors.New("Unknown driver: " + driverName)
	}
}
