package timezone

import (
	"errors"
	"strings"
	"time"
	_ "time/tzdata"
)

const Default = "Asia/Jakarta"

var ErrInvalid = errors.New("timezone must be a valid IANA timezone")

// Normalize applies the outlet default and validates explicit choices.
func Normalize(input *string) (string, error) {
	zone := ""
	if input != nil {
		zone = strings.TrimSpace(*input)
	}
	if zone == "" {
		return Default, nil
	}
	if zone == "Local" || len(zone) > 255 {
		return "", ErrInvalid
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return "", ErrInvalid
	}
	return zone, nil
}
