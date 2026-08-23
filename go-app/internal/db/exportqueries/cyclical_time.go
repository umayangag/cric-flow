package exportqueries

import (
	"math"
	"strconv"
	"time"
)

// cyclicalMonthSin returns sin(2π × month / 12) for the given time.
func cyclicalMonthSin(t time.Time) string {
	return strconv.FormatFloat(math.Sin(2*math.Pi*float64(t.Month())/12.0), 'g', -1, 64)
}

// cyclicalMonthCos returns cos(2π × month / 12) for the given time.
func cyclicalMonthCos(t time.Time) string {
	return strconv.FormatFloat(math.Cos(2*math.Pi*float64(t.Month())/12.0), 'g', -1, 64)
}

// cyclicalDowSin returns sin(2π × weekday / 7) for the given time.
func cyclicalDowSin(t time.Time) string {
	return strconv.FormatFloat(math.Sin(2*math.Pi*float64(t.Weekday())/7.0), 'g', -1, 64)
}

// cyclicalDowCos returns cos(2π × weekday / 7) for the given time.
func cyclicalDowCos(t time.Time) string {
	return strconv.FormatFloat(math.Cos(2*math.Pi*float64(t.Weekday())/7.0), 'g', -1, 64)
}
