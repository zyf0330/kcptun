package main

import (
	"fmt"
	"net"

	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/generic"
	"github.com/xtaci/tcpraw"
)

var dialCount uint64

// createConn: optional
func dial(config *Config, block kcp.BlockCrypt, createConn func(remoteAddr string) (net.PacketConn, error)) (*kcp.UDPSession, error) {
	defer func() {
		dialCount++
	}()

	mp, err := generic.ParseMultiPort(config.RemoteAddr)
	if err != nil {
		return nil, err
	}

	remoteAddr := fmt.Sprintf("%v:%v", mp.Host, uint64(mp.MinPort)+dialCount%uint64(mp.MaxPort-mp.MinPort+1))

	if config.TCP {
		var tcpConn net.PacketConn
		if createConn != nil {
			if conn, err := createConn(remoteAddr); err != nil {
				return nil, errors.Wrap(err, "tcp createConn()")
			} else {
				tcpConn = conn
			}
		} else {
			if conn, err := tcpraw.Dial("tcp", remoteAddr); err != nil {
				return nil, errors.Wrap(err, "tcpraw.Dial()")
			} else {
				tcpConn = conn
			}
		}
		return kcp.NewConn(remoteAddr, block, config.DataShard, config.ParityShard, tcpConn)
	}

	if createConn != nil {
		if c, err := createConn(remoteAddr); err != nil {
			return nil, err
		} else {
			return kcp.NewConn(remoteAddr, block, config.DataShard, config.ParityShard, c)
		}
	} else {
		return kcp.DialWithOptions(remoteAddr, block, config.DataShard, config.ParityShard)
	}
}
