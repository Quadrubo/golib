package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Bytes is a byte count. A config gives it as a plain number, or as a string
// carrying one of the binary units B, KiB, MiB and GiB, such as 128MiB.
type Bytes int64

var byteUnits = map[string]int64{
	"B":   1,
	"KiB": 1 << 10,
	"MiB": 1 << 20,
	"GiB": 1 << 30,
}

func (b *Bytes) UnmarshalText(text []byte) error {
	value := string(text)

	cut := strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' })
	if cut == -1 {
		cut = len(value)
	}

	number, err := strconv.ParseInt(value[:cut], 10, 64)
	if err != nil {
		return fmt.Errorf("unsupported byte count %q", value)
	}

	factor := int64(1)
	if unit := value[cut:]; unit != "" {
		declared, ok := byteUnits[unit]
		if !ok {
			return fmt.Errorf("unsupported byte unit %q", unit)
		}

		factor = declared
	}

	if number > math.MaxInt64/factor {
		return fmt.Errorf("unsupported byte count %q", value)
	}

	*b = Bytes(number * factor)

	return nil
}
