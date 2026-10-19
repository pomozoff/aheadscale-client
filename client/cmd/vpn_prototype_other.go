//go:build !windows || !windowsvpnprototype

package cmd

import "google.golang.org/grpc"

func prototypeServerOptions() []grpc.ServerOption { return nil }
