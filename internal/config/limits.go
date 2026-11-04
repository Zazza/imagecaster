package config

import "fmt"

const (
	MaxImageBytes     = 128 << 20
	MaxImageBase64Len = (MaxImageBytes + 2) / 3 * 4
)

func ImageTooLarge(what string) error {
	return fmt.Errorf("%s too large (max %d MB)", what, MaxImageBytes>>20)
}
