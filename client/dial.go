// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/std"
	"github.com/xtaci/tcpraw"
)

type ConnProvider struct {
	createConn func(isTCP bool, remoteAddr string) (net.PacketConn, error)
}

// dial connects to the remote address
func dial(config *Config, block kcp.BlockCrypt, connProvider *ConnProvider) (*kcp.UDPSession, error) {
	mp, err := std.ParseMultiPort(config.RemoteAddr)
	if err != nil {
		return nil, err
	}

	// generate a random port
	var randport uint64
	err = binary.Read(rand.Reader, binary.LittleEndian, &randport)
	if err != nil {
		return nil, err
	}
	remoteAddr := fmt.Sprintf("%v:%v", mp.Host, uint64(mp.MinPort)+randport%uint64(mp.MaxPort-mp.MinPort+1))

	// emulate TCP connection
	if config.TCP {
		var tcpConn net.PacketConn
		if connProvider.createConn != nil {
			if conn, err := connProvider.createConn(true, remoteAddr); err != nil {
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

	if connProvider.createConn != nil {
		if c, err := connProvider.createConn(false, remoteAddr); err != nil {
			return nil, err
		} else {
			return kcp.NewConn(remoteAddr, block, config.DataShard, config.ParityShard, c)
		}
	} else {
	    // default UDP connection
		return kcp.DialWithOptions(remoteAddr, block, config.DataShard, config.ParityShard)
	}
}
