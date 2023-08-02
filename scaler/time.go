package scaler

import (
	"fmt"
	"strconv"
	"time"
)

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

func CalculateRemainingCooldown(cooldown time.Duration, lastScaleTime time.Time) int {
	remainingCooldown := cooldown - time.Since(lastScaleTime)
	if remainingCooldown > 0 {
		return int(remainingCooldown.Seconds())
	}
	return 0
}
