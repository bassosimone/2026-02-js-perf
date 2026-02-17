//
// SPDX-License-Identifier: GPL-3.0-or-later
//
// Adapted from: https://github.com/ooni/probe-cli/blob/v3.20.0/internal/humanize/humanize.go
//

package humanize

import "fmt"

// IEC formats a value using IEC (base-1024) prefixes.
func IEC(value float64, unit string) string {
	switch {
	case value >= 1<<30:
		return fmt.Sprintf("%.1f Gi%s", value/(1<<30), unit)
	case value >= 1<<20:
		return fmt.Sprintf("%.1f Mi%s", value/(1<<20), unit)
	case value >= 1<<10:
		return fmt.Sprintf("%.1f Ki%s", value/(1<<10), unit)
	default:
		return fmt.Sprintf("%.0f %s", value, unit)
	}
}

// SI formats a value using SI (base-10) prefixes.
func SI(value float64, unit string) string {
	switch {
	case value >= 1e9:
		return fmt.Sprintf("%.1f G%s", value/1e9, unit)
	case value >= 1e6:
		return fmt.Sprintf("%.1f M%s", value/1e6, unit)
	case value >= 1e3:
		return fmt.Sprintf("%.1f k%s", value/1e3, unit)
	default:
		return fmt.Sprintf("%.0f %s", value, unit)
	}
}
