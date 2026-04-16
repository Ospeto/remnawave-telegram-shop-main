package utils

import (
	"strconv"
	"strings"
)

func MaskHalfInt(input int) string {
	return MaskHalf(strconv.Itoa(input))
}

func MaskHalfInt64(input int64) string {
	return MaskHalf(strconv.FormatInt(input, 10))
}

func MaskHalf(input string) string {
	if input == "" {
		return input
	}
	if len(input) < 2 {
		return input
	}
	length := len(input)
	visibleLength := length / 2
	maskedLength := length - visibleLength
	return input[:visibleLength] + strings.Repeat("*", maskedLength)
}

func MaskTail(input string, visible int) string {
	if input == "" {
		return input
	}
	if visible <= 0 {
		return strings.Repeat("*", len(input))
	}
	if len(input) <= visible {
		return strings.Repeat("*", len(input))
	}
	return strings.Repeat("*", len(input)-visible) + input[len(input)-visible:]
}

func FirstToken(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
