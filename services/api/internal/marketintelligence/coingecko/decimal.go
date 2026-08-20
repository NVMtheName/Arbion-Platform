package coingecko

import (
	"regexp"
	"strconv"
	"strings"
)

var numberPattern = regexp.MustCompile(`^(-?)([0-9]+)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)

// normalizeDecimal expands JSON exponent notation without converting through a
// floating-point type. The output is an exact, bounded canonical decimal.
func normalizeDecimal(value string, signed bool) (string, bool) {
	match := numberPattern.FindStringSubmatch(value)
	if match == nil || (!signed && match[1] == "-") {
		return "", false
	}
	exponent := 0
	var err error
	if match[4] != "" {
		exponent, err = strconv.Atoi(match[4])
		if err != nil || exponent < -100 || exponent > 100 {
			return "", false
		}
	}
	digits := match[2] + match[3]
	decimalPosition := len(match[2]) + exponent
	var normalized string
	switch {
	case decimalPosition <= 0:
		normalized = "0." + strings.Repeat("0", -decimalPosition) + digits
	case decimalPosition >= len(digits):
		normalized = digits + strings.Repeat("0", decimalPosition-len(digits))
	default:
		normalized = digits[:decimalPosition] + "." + digits[decimalPosition:]
	}
	parts := strings.SplitN(normalized, ".", 2)
	parts[0] = strings.TrimLeft(parts[0], "0")
	if parts[0] == "" {
		parts[0] = "0"
	}
	if len(parts) == 2 {
		parts[1] = strings.TrimRight(parts[1], "0")
		if parts[1] == "" {
			normalized = parts[0]
		} else {
			normalized = parts[0] + "." + parts[1]
		}
	} else {
		normalized = parts[0]
	}
	if match[1] == "-" && normalized != "0" {
		normalized = "-" + normalized
	}
	if len(normalized) == 0 || len(normalized) > 128 {
		return "", false
	}
	return normalized, true
}
