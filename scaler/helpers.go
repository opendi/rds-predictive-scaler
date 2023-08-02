package scaler

import "strings"

// Helper function to return the minimum of two integers
func minInt(a, b uint) uint {
	if a < b {
		return a
	}
	return b
}

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
