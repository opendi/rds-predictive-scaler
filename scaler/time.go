package scaler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func IsScaleOutHour(hour int, scaleOutHours []int) bool {
	for _, h := range scaleOutHours {
		if hour == h {
			return true
		}
	}
	return false
}

func ParseScaleOutHours(scaleOutHoursStr string) ([]int, error) {
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

func splitAndTrimStrings(input, sep string) []string {
	items := strings.Split(input, sep)
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}

func CalculateRemainingCooldown(cooldown time.Duration, lastScaleTime time.Time) int {
	remainingCooldown := cooldown - time.Since(lastScaleTime)
	if remainingCooldown > 0 {
		return int(remainingCooldown.Seconds())
	}
	return 0
}
