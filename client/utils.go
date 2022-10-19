//go:build !android
// +build !android

package main

import "github.com/xtaci/kcp-go/v5"

func DialKCP(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	return dial(config, block, &ConnProvider{})
}

func log_init() {
}
