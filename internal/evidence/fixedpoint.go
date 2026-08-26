package evidence

import (
	"errors"
	"math/big"
	"strings"
)

// Fixed-point parse errors. These are stable and map to business error codes.
var (
	ErrInvalidPrecision = errors.New("fixedpoint: invalid precision")
	ErrInvalidLength    = errors.New("fixedpoint: invalid length")
	ErrInvalidFormat    = errors.New("fixedpoint: invalid format")
	ErrPrecision        = errors.New("fixedpoint: precision exceeded")
	ErrOverflow         = errors.New("fixedpoint: value overflows int64")
)

// maxPrecision bounds the supported decimal places (an int64 scaled value must
// stay within the signed 64-bit range).
const maxPrecision = 18

// ParseFixed parses a decimal text into a signed 64-bit fixed-point integer
// scaled by the given precision. It validates length, sign, digit set, decimal
// precision, and int64 overflow; any failure yields a zero value and an error
// so no derived evidence is produced from invalid input.
func ParseFixed(text string, precision int) (int64, error) {
	if precision < 0 || precision > maxPrecision {
		return 0, ErrInvalidPrecision
	}
	if text == "" || len(text) > 96 {
		return 0, ErrInvalidLength
	}

	negative := false
	s := text
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		negative = true
		s = s[1:]
	}
	if s == "" {
		return 0, ErrInvalidFormat
	}

	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, fracPart = s[:i], s[i+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return 0, ErrInvalidFormat
		}
	}
	if intPart == "" {
		intPart = "0"
	}
	if !isDigits(intPart) || !isDigits(fracPart) {
		return 0, ErrInvalidFormat
	}
	if len(fracPart) > precision {
		return 0, ErrPrecision
	}

	digits := intPart + fracPart + strings.Repeat("0", precision-len(fracPart))
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0, ErrInvalidFormat
	}
	if negative {
		n.Neg(n)
	}
	if !n.IsInt64() {
		return 0, ErrOverflow
	}
	return n.Int64(), nil
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
