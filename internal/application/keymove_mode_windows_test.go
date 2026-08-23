//go:build windows

package application

import "testing"

func markKeyMode(*testing.T, string) {}

func assertKeyModeSurvived(*testing.T, string) {}
