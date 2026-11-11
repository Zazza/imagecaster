//go:build !linux

package main

import "github.com/wailsapp/wails/v2/pkg/options"

func windowBackground() *options.RGBA { return nil }
