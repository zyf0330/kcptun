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
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/pbkdf2"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/std"
	"github.com/xtaci/qpp"
	"github.com/xtaci/smux"
)

const (
	// SALT is use for pbkdf2 key expansion
	SALT = "kcp-go"
	// maximum supported smux version
	maxSmuxVer = 2
)

var VpnMode = false

// VERSION is injected by buildflags
var VERSION = "SELFBUILD"

func main() {
	if VERSION == "SELFBUILD" {
		// add more log flags for debugging
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	myApp := cli.NewApp()
	myApp.Name = "kcptun"
	myApp.Usage = "client(with SMUX)"
	myApp.Version = VERSION
	myApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "localaddr,l",
			Value: ":12948",
			Usage: "local listen address",
		},
		cli.StringFlag{
			Name:  "remoteaddr, r",
			Value: "vps:29900",
			Usage: `kcp server address, eg: "IP:29900" a for single port, "IP:minport-maxport" for port range`,
		},
		cli.StringFlag{
			Name:   "key",
			Value:  "it's a secrect",
			Usage:  "pre-shared secret between client and server",
			EnvVar: "KCPTUN_KEY",
		},
		cli.StringFlag{
			Name:  "crypt",
			Value: "aes",
			Usage: "aes, aes-128, aes-192, salsa20, blowfish, twofish, cast5, 3des, tea, xtea, xor, sm4, none, null",
		},
		cli.StringFlag{
			Name:  "mode",
			Value: "fast",
			Usage: "profiles: fast3, fast2, fast, normal, manual",
		},
		cli.BoolFlag{
			Name:  "QPP",
			Usage: "enable Quantum Permutation Pads(QPP)",
		},
		cli.IntFlag{
			Name:  "QPPCount",
			Value: 61,
			Usage: "the prime number of pads to use for QPP: The more pads you use, the more secure the encryption. Each pad requires 256 bytes.",
		},
		cli.IntFlag{
			Name:  "conn",
			Value: 1,
			Usage: "set num of UDP connections to server",
		},
		cli.IntFlag{
			Name:  "autoexpire",
			Value: 0,
			Usage: "set auto expiration time(in seconds) for a single UDP connection, 0 to disable",
		},
		cli.IntFlag{
			Name:  "scavengettl",
			Value: 600,
			Usage: "set how long an expired connection can live (in seconds)",
		},
		cli.IntFlag{
			Name:  "mtu",
			Value: 1350,
			Usage: "set maximum transmission unit for UDP packets",
		},
		cli.IntFlag{
			Name:  "sndwnd",
			Value: 128,
			Usage: "set send window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "rcvwnd",
			Value: 512,
			Usage: "set receive window size(num of packets)",
		},
		cli.IntFlag{
			Name:  "datashard,ds",
			Value: 10,
			Usage: "set reed-solomon erasure coding - datashard",
		},
		cli.IntFlag{
			Name:  "parityshard,ps",
			Value: 3,
			Usage: "set reed-solomon erasure coding - parityshard",
		},
		cli.IntFlag{
			Name:  "dscp",
			Value: 0,
			Usage: "set DSCP(6bit)",
		},
		cli.BoolFlag{
			Name:  "nocomp",
			Usage: "disable compression",
		},
		cli.BoolFlag{
			Name:   "acknodelay",
			Usage:  "flush ack immediately when a packet is received",
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "nodelay",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "interval",
			Value:  50,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "resend",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:   "nc",
			Value:  0,
			Hidden: true,
		},
		cli.IntFlag{
			Name:  "sockbuf",
			Value: 4194304, // socket buffer size in bytes
			Usage: "per-socket buffer in bytes",
		},
		cli.IntFlag{
			Name:  "smuxver",
			Value: 1,
			Usage: "specify smux version, available 1,2",
		},
		cli.IntFlag{
			Name:  "smuxbuf",
			Value: 4194304,
			Usage: "the overall de-mux buffer in bytes",
		},
		cli.IntFlag{
			Name:  "streambuf",
			Value: 2097152,
			Usage: "per stream receive buffer in bytes, smux v2+",
		},
		cli.IntFlag{
			Name:  "keepalive",
			Value: 10, // nat keepalive interval in seconds
			Usage: "seconds between heartbeats",
		},
		cli.StringFlag{
			Name:  "snmplog",
			Value: "",
			Usage: "collect snmp to file, aware of timeformat in golang, like: ./snmp-20060102.log",
		},
		cli.IntFlag{
			Name:  "snmpperiod",
			Value: 60,
			Usage: "snmp collect period, in seconds",
		},
		cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "specify a log file to output, default goes to stderr",
		},
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "to suppress the 'stream open/close' messages",
		},
		cli.BoolFlag{
			Name:  "tcp",
			Usage: "to emulate a TCP connection(linux)",
		},
		cli.StringFlag{
			Name:  "c",
			Value: "", // when the value is not empty, the config path must exists
			Usage: "config from json file, which will override the command from shell",
		},
		cli.BoolFlag{
			Name:  "pprof",
			Usage: "start profiling server on :6060",
		},
		cli.BoolFlag{
			Name:  "fast-open",
			Usage: "Dummy flag, doesn't really do anything",
		},
		cli.BoolFlag{
			Name:  "V",
			Usage: "Enable VPN mode for shadowsocks-android",
		},
	}
	myApp.Action = func(c *cli.Context) error {
		config := Config{}

		config.LocalAddr = c.String("localaddr")
		config.RemoteAddr = c.String("remoteaddr")
		config.Key = c.String("key")
		config.Crypt = c.String("crypt")
		config.Mode = c.String("mode")
		config.Conn = c.Int("conn")
		config.AutoExpire = c.Int("autoexpire")
		config.ScavengeTTL = c.Int("scavengettl")
		config.MTU = c.Int("mtu")
		config.SndWnd = c.Int("sndwnd")
		config.RcvWnd = c.Int("rcvwnd")
		config.DataShard = c.Int("datashard")
		config.ParityShard = c.Int("parityshard")
		config.DSCP = c.Int("dscp")
		config.NoComp = c.Bool("nocomp")
		config.AckNodelay = c.Bool("acknodelay")
		config.NoDelay = c.Int("nodelay")
		config.Interval = c.Int("interval")
		config.Resend = c.Int("resend")
		config.NoCongestion = c.Int("nc")
		config.SockBuf = c.Int("sockbuf")
		config.SmuxBuf = c.Int("smuxbuf")
		config.StreamBuf = c.Int("streambuf")
		config.SmuxVer = c.Int("smuxver")
		config.KeepAlive = c.Int("keepalive")
		config.Log = c.String("log")
		config.SnmpLog = c.String("snmplog")
		config.SnmpPeriod = c.Int("snmpperiod")
		config.Quiet = c.Bool("quiet")
		config.TCP = c.Bool("tcp")
		config.Pprof = c.Bool("pprof")
		config.QPP = c.Bool("QPP")
		config.QPPCount = c.Int("QPPCount")
		config.Vpn = c.Bool("V")

		if c.String("c") != "" {
			err := parseJSONConfig(&config, c.String("c"))
			checkError(err)
		}

		opts, err := parseEnv()
		if err == nil {
			fmt.Printf("test")
			if c, b := opts.Get("localaddr"); b {
				config.LocalAddr = c
			}
			if c, b := opts.Get("remoteaddr"); b {
				config.RemoteAddr = c
			}
			if c, b := opts.Get("key"); b {
				config.Key = c
			}
			if c, b := opts.Get("crypt"); b {
				config.Crypt = c
			}
			if c, b := opts.Get("mode"); b {
				config.Mode = c
			}
			if c, b := opts.Get("conn"); b {
				if conn, err := strconv.Atoi(c); err == nil {
					config.Conn = conn
				}
			}
			if c, b := opts.Get("autoexpire"); b {
				if autoexpire, err := strconv.Atoi(c); err == nil {
					config.AutoExpire = autoexpire
				}
			}
			if c, b := opts.Get("scavengettl"); b {
				if scavengettl, err := strconv.Atoi(c); err == nil {
					config.ScavengeTTL = scavengettl
				}
			}
			if c, b := opts.Get("mtu"); b {
				if mtu, err := strconv.Atoi(c); err == nil {
					config.MTU = mtu
				}
			}
			if c, b := opts.Get("sndwnd"); b {
				if sndwnd, err := strconv.Atoi(c); err == nil {
					config.SndWnd = sndwnd
				}
			}
			if c, b := opts.Get("rcvwnd"); b {
				if rcvwnd, err := strconv.Atoi(c); err == nil {
					config.RcvWnd = rcvwnd
				}
			}
			if c, b := opts.Get("datashard"); b {
				if datashard, err := strconv.Atoi(c); err == nil {
					config.DataShard = datashard
				}
			}
			if c, b := opts.Get("parityshard"); b {
				if parityshard, err := strconv.Atoi(c); err == nil {
					config.ParityShard = parityshard
				}
			}
			if c, b := opts.Get("dscp"); b {
				if dscp, err := strconv.Atoi(c); err == nil {
					config.DSCP = dscp
				}
			}
			if c, b := opts.Get("nocomp"); b {
				if nocomp, err := strconv.ParseBool(c); err == nil {
					config.NoComp = nocomp
				}
			}
			if c, b := opts.Get("acknodelay"); b {
				if acknodelay, err := strconv.ParseBool(c); err == nil {
					config.AckNodelay = acknodelay
				}
			}
			if c, b := opts.Get("nodelay"); b {
				if nodelay, err := strconv.Atoi(c); err == nil {
					config.NoDelay = nodelay
				}
			}
			if c, b := opts.Get("interval"); b {
				if interval, err := strconv.Atoi(c); err == nil {
					config.Interval = interval
				}
			}
			if c, b := opts.Get("resend"); b {
				if resend, err := strconv.Atoi(c); err == nil {
					config.Resend = resend
				}
			}
			if c, b := opts.Get("nc"); b {
				if nc, err := strconv.Atoi(c); err == nil {
					config.NoCongestion = nc
				}
			}
			if c, b := opts.Get("sockbuf"); b {
				if sockbuf, err := strconv.Atoi(c); err == nil {
					config.SockBuf = sockbuf
				}
			}
			if c, b := opts.Get("smuxver"); b {
				if smuxver, err := strconv.Atoi(c); err == nil {
					config.SmuxVer = smuxver
				}
			}
			if c, b := opts.Get("smuxbuf"); b {
				if smuxbuf, err := strconv.Atoi(c); err == nil {
					config.SmuxBuf = smuxbuf
				}
			}
			if c, b := opts.Get("streambuf"); b {
				if streambuf, err := strconv.Atoi(c); err == nil {
					config.StreamBuf = streambuf
				}
			}
			if c, b := opts.Get("keepalive"); b {
				if keepalive, err := strconv.Atoi(c); err == nil {
					config.KeepAlive = keepalive
				}
			}
			if c, b := opts.Get("log"); b {
				config.Log = c
			}
			if c, b := opts.Get("snmplog"); b {
				config.SnmpLog = c
			}
			if c, b := opts.Get("snmpperiod"); b {
				if snmpperiod, err := strconv.Atoi(c); err == nil {
					config.SnmpPeriod = snmpperiod
				}
			}
			if c, b := opts.Get("quiet"); b {
				if quiet, err := strconv.ParseBool(c); err == nil {
					config.Quiet = quiet
				}
			}
			if c, b := opts.Get("tcp"); b {
				if tcp, err := strconv.ParseBool(c); err == nil {
					config.TCP = tcp
				}
			}
			if c, b := opts.Get("qpp"); b {
				if qpp, err := strconv.ParseBool(c); err == nil {
					config.QPP = qpp
				}
			}
			if c, b := opts.Get("qppcount"); b {
				if qppcount, err := strconv.Atoi(c); err == nil {
					config.QPPCount = qppcount
				}
			}
			if _, b := opts.Get("__android_vpn"); b {
				config.Vpn = true
			}
		}

		// log redirect
		if config.Log != "" {
			f, err := os.OpenFile(config.Log, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
			checkError(err)
			defer f.Close()
			log.SetOutput(f)
		}

		switch config.Mode {
		case "normal":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 40, 2, 1
		case "fast":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 0, 30, 2, 1
		case "fast2":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 20, 2, 1
		case "fast3":
			config.NoDelay, config.Interval, config.Resend, config.NoCongestion = 1, 10, 2, 1
		}

		log.Println("version:", VERSION)
		var listener net.Listener
		var isUnix bool
		if _, _, err := net.SplitHostPort(config.LocalAddr); err != nil {
			isUnix = true
		}
		if isUnix {
			addr, err := net.ResolveUnixAddr("unix", config.LocalAddr)
			checkError(err)
			listener, err = net.ListenUnix("unix", addr)
			checkError(err)
		} else {
			addr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
			checkError(err)
			listener, err = net.ListenTCP("tcp", addr)
			checkError(err)
		}

		log_init()

		log.Println("smux version:", config.SmuxVer)
		log.Println("listening on:", listener.Addr())
		log.Println("encryption:", config.Crypt)
		log.Println("QPP:", config.QPP)
		log.Println("QPP Count:", config.QPPCount)
		log.Println("nodelay parameters:", config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
		log.Println("remote address:", config.RemoteAddr)
		log.Println("sndwnd:", config.SndWnd, "rcvwnd:", config.RcvWnd)
		log.Println("compression:", !config.NoComp)
		log.Println("mtu:", config.MTU)
		log.Println("datashard:", config.DataShard, "parityshard:", config.ParityShard)
		log.Println("acknodelay:", config.AckNodelay)
		log.Println("dscp:", config.DSCP)
		log.Println("sockbuf:", config.SockBuf)
		log.Println("smuxbuf:", config.SmuxBuf)
		log.Println("streambuf:", config.StreamBuf)
		log.Println("keepalive:", config.KeepAlive)
		log.Println("conn:", config.Conn)
		log.Println("autoexpire:", config.AutoExpire)
		log.Println("scavengettl:", config.ScavengeTTL)
		log.Println("snmplog:", config.SnmpLog)
		log.Println("snmpperiod:", config.SnmpPeriod)
		log.Println("quiet:", config.Quiet)
		log.Println("tcp:", config.TCP)
		log.Println("pprof:", config.Pprof)
		log.Println("vpn:", config.Vpn)

		VpnMode = config.Vpn

		// QPP parameters check
		if config.QPP {
			minSeedLength := qpp.QPPMinimumSeedLength(8)
			if len(config.Key) < minSeedLength {
				color.Red("QPP Warning: 'key' has size of %d bytes, required %d bytes at least", len(config.Key), minSeedLength)
			}

			minPads := qpp.QPPMinimumPads(8)
			if config.QPPCount < minPads {
				color.Red("QPP Warning: QPPCount %d, required %d at least", config.QPPCount, minPads)
			}

			if new(big.Int).GCD(nil, nil, big.NewInt(int64(config.QPPCount)), big.NewInt(8)).Int64() != 1 {
				color.Red("QPP Warning: QPPCount %d, choose a prime number for security", config.QPPCount)
			}
		}

		// SMUX Version check
		if config.SmuxVer > maxSmuxVer {
			log.Fatal("unsupported smux version:", config.SmuxVer)
		}

		log.Println("initiating key derivation")
		pass := pbkdf2.Key([]byte(config.Key), []byte(SALT), 4096, 32, sha1.New)
		log.Println("key derivation done")
		var block kcp.BlockCrypt
		switch config.Crypt {
		case "null":
			block = nil
		case "sm4":
			block, _ = kcp.NewSM4BlockCrypt(pass[:16])
		case "tea":
			block, _ = kcp.NewTEABlockCrypt(pass[:16])
		case "xor":
			block, _ = kcp.NewSimpleXORBlockCrypt(pass)
		case "none":
			block, _ = kcp.NewNoneBlockCrypt(pass)
		case "aes-128":
			block, _ = kcp.NewAESBlockCrypt(pass[:16])
		case "aes-192":
			block, _ = kcp.NewAESBlockCrypt(pass[:24])
		case "blowfish":
			block, _ = kcp.NewBlowfishBlockCrypt(pass)
		case "twofish":
			block, _ = kcp.NewTwofishBlockCrypt(pass)
		case "cast5":
			block, _ = kcp.NewCast5BlockCrypt(pass[:16])
		case "3des":
			block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
		case "xtea":
			block, _ = kcp.NewXTEABlockCrypt(pass[:16])
		case "salsa20":
			block, _ = kcp.NewSalsa20BlockCrypt(pass)
		default:
			config.Crypt = "aes"
			block, _ = kcp.NewAESBlockCrypt(pass)
		}

		createConn := func() (*smux.Session, error) {
			kcpconn, err := DialKCP(&config, block)
			if err != nil {
				return nil, errors.Wrap(err, "dial()")
			}
			kcpconn.SetStreamMode(true)
			kcpconn.SetWriteDelay(false)
			kcpconn.SetNoDelay(config.NoDelay, config.Interval, config.Resend, config.NoCongestion)
			kcpconn.SetWindowSize(config.SndWnd, config.RcvWnd)
			kcpconn.SetMtu(config.MTU)
			kcpconn.SetACKNoDelay(config.AckNodelay)

			if err := kcpconn.SetDSCP(config.DSCP); err != nil {
				log.Println("SetDSCP:", err)
			}
			if err := kcpconn.SetReadBuffer(config.SockBuf); err != nil {
				log.Println("SetReadBuffer:", err)
			}
			if err := kcpconn.SetWriteBuffer(config.SockBuf); err != nil {
				log.Println("SetWriteBuffer:", err)
			}
			log.Println("smux version:", config.SmuxVer, "on connection:", kcpconn.LocalAddr(), "->", kcpconn.RemoteAddr())
			smuxConfig := smux.DefaultConfig()
			smuxConfig.Version = config.SmuxVer
			smuxConfig.MaxReceiveBuffer = config.SmuxBuf
			smuxConfig.MaxStreamBuffer = config.StreamBuf
			smuxConfig.KeepAliveInterval = time.Duration(config.KeepAlive) * time.Second

			if err := smux.VerifyConfig(smuxConfig); err != nil {
				log.Fatalf("%+v", err)
			}

			// stream multiplex
			var session *smux.Session
			if config.NoComp {
				session, err = smux.Client(kcpconn, smuxConfig)
			} else {
				session, err = smux.Client(std.NewCompStream(kcpconn), smuxConfig)
			}
			if err != nil {
				return nil, errors.Wrap(err, "createConn()")
			}
			return session, nil
		}

		// wait until a connection is ready
		waitConn := func() *smux.Session {
			for {
				if session, err := createConn(); err == nil {
					return session
				} else {
					log.Println("re-connecting:", err)
					time.Sleep(time.Second)
				}
			}
		}

		// start snmp logger
		go std.SnmpLogger(config.SnmpLog, config.SnmpPeriod)

		// start pprof
		if config.Pprof {
			go http.ListenAndServe(":6060", nil)
		}

		// start scavenger
		chScavenger := make(chan timedSession, 128)
		go scavenger(chScavenger, &config)

		// start parent process monitor
		go parentMonitor(3)

		// start listener
		numconn := uint16(config.Conn)
		muxes := make([]timedSession, numconn)
		rr := uint16(0)

		// create shared QPP
		var _Q_ *qpp.QuantumPermutationPad
		if config.QPP {
			_Q_ = qpp.NewQPP([]byte(config.Key), uint16(config.QPPCount))
		}

		for {
			p1, err := listener.Accept()
			if err != nil {
				log.Fatalf("%+v", err)
			}
			idx := rr % numconn

			// do auto expiration && reconnection
			if muxes[idx].session == nil || muxes[idx].session.IsClosed() ||
				(config.AutoExpire > 0 && time.Now().After(muxes[idx].expiryDate)) {
				muxes[idx].session = waitConn()
				muxes[idx].expiryDate = time.Now().Add(time.Duration(config.AutoExpire) * time.Second)
				if config.AutoExpire > 0 { // only when autoexpire set
					chScavenger <- muxes[idx]
				}
			}

			go handleClient(_Q_, []byte(config.Key), muxes[idx].session, p1, config.Quiet)
			rr++
		}
	}
	myApp.Run(os.Args)
}

func parentMonitor(interval int) {
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	pid := os.Getppid()
	for {
		select {
		case <-ticker.C:
			curpid := os.Getppid()
			if curpid != pid {
				os.Exit(1)
			}
		}
	}
}

// handleClient aggregates connection p1 on mux
func handleClient(_Q_ *qpp.QuantumPermutationPad, seed []byte, session *smux.Session, p1 net.Conn, quiet bool) {
	logln := func(v ...interface{}) {
		if !quiet {
			log.Println(v...)
		}
	}

	// handles transport layer
	defer p1.Close()
	p2, err := session.OpenStream()
	if err != nil {
		logln(err)
		return
	}
	defer p2.Close()

	logln("stream opened", "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	defer logln("stream closed", "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))

	var s1, s2 io.ReadWriteCloser = p1, p2
	// if QPP is enabled, create QPP read write closer
	if _Q_ != nil {
		// replace s2 with QPP port
		s2 = std.NewQPPPort(p2, _Q_, seed)
	}

	// stream layer
	err1, err2 := std.Pipe(s1, s2)

	// handles transport layer errors
	if err1 != nil && err1 != io.EOF {
		logln("pipe:", err1, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	}
	if err2 != nil && err2 != io.EOF {
		logln("pipe:", err2, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
	}
}

func checkError(err error) {
	if err != nil {
		log.Printf("%+v\n", err)
		os.Exit(-1)
	}
}

// timedSession is a wrapper for smux.Session with expiry date
type timedSession struct {
	session    *smux.Session
	expiryDate time.Time
}

// scavenger goroutine is used to close expired sessions
func scavenger(ch chan timedSession, config *Config) {
	// When AutoExpire is set to 0 (default), sessionList will keep empty.
	// Then this routine won't need to do anything; thus just terminate it.
	if config.AutoExpire <= 0 {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var sessionList []timedSession
	for {
		select {
		case item := <-ch:
			sessionList = append(sessionList, timedSession{
				item.session,
				item.expiryDate.Add(time.Duration(config.ScavengeTTL) * time.Second)})
		case <-ticker.C:
			if len(sessionList) == 0 {
				continue
			}

			var newList []timedSession
			for k := range sessionList {
				s := sessionList[k]
				if s.session.IsClosed() {
					log.Println("scavenger: session normally closed:", s.session.LocalAddr())
				} else if time.Now().After(s.expiryDate) {
					s.session.Close()
					log.Println("scavenger: session closed due to ttl:", s.session.LocalAddr())
				} else {
					newList = append(newList, sessionList[k])
				}
			}
			sessionList = newList
		}
	}
}
