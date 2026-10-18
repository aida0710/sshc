//go:build !windows

package main

import "context"

func webUpdateServiceStarter() func(context.Context, string) (bool, error) { return nil }
