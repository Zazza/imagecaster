package preset

const (
	FastModeOn  = "on"
	FastModeOff = "off"
)

func NormalizeFastMode(v string) string {
	switch v {
	case FastModeOn, FastModeOff:
		return v
	}
	return ""
}
