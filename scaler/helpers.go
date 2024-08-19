package scaler

import (
	"fmt"
	"strconv"
	"strings"
)

func containsString(list []string, str string) bool {
	for _, s := range list {
		if s == str {
			return true
		}
	}
	return false
}

func splitAndTrimStrings(input, sep string) []string {
	items := strings.Split(input, sep)
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}

func isBoostHour(hour int, scaleOutHours []int) bool {
	for _, h := range scaleOutHours {
		if hour == h {
			return true
		}
	}
	return false
}

func parseBoostHours(scaleOutHoursStr string) ([]int, error) {
	if scaleOutHoursStr == "" {
		return nil, nil // Return nil to indicate no boost hours specified
	}

	hoursStr := splitAndTrimStrings(scaleOutHoursStr, ",")
	scaleOutHours := make([]int, 0, len(hoursStr))
	for _, hourStr := range hoursStr {
		hour, err := strconv.Atoi(hourStr)
		if err != nil {
			return nil, fmt.Errorf("invalid hour: %s", hourStr)
		}
		scaleOutHours = append(scaleOutHours, hour)
	}
	return scaleOutHours, nil
}
