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

func IsScaleInHour(hour int, scaleOutHours []int) bool {
	return !IsScaleOutHour(hour, scaleOutHours)
}

func SleepUntilNextHour() {
	// Get the current time
	now := time.Now()

	// Calculate the duration until the next hour
	nextHour := now.Truncate(time.Hour).Add(time.Hour)
	sleepDuration := nextHour.Sub(now)

	// Output the sleep duration
	fmt.Printf("Sleeping until the next hour at: %s\n", nextHour.Format("15:04:05"))

	// Sleep until the next hour
	time.Sleep(sleepDuration)
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
