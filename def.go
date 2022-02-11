package enet

import (
	"fmt"
	"time"

	"go.uber.org/atomic"
)

const (
	ConnEstablishType uint32 = iota
	ConnRecvMsgType
	ConnCloseType
)

type SessionType uint32

const (
	SessConnectType SessionType = iota
	SessListenType
)

var GSendQps atomic.Int64 = *atomic.NewInt64(0)
var GRecvQps atomic.Int64 = *atomic.NewInt64(0)

const (
	ConnEstablishState uint32 = iota
	ConnClosedState
)

const (
	MsgSuccess uint32 = 0
	MsgFail    uint32 = 1
)

const ConnWriterSleepLoopCount = 10000

const (
	NetChannelMaxSize     = 10000000
	NetMaxConnectSize     = 65535
	PackageDefaultMaxSize = 1024 * 64

	SmallSendBuffMaxSize = 1000    //c-s
	BigSendBuffMaxSize   = 1000000 //s-s
)

const (
	IsFreeConnectState uint32 = 0
	IsConnectingState  uint32 = 1
)

func getMillsecond() int64 {
	return time.Now().UnixNano() / 1e6
}

func getSecond() int64 {
	return time.Now().UnixNano() / 1e9
}

const NetMajorVersion = 1
const NetMinorVersion = 1

type NetVersion struct {
}

func (n *NetVersion) GetVersion() string {
	return fmt.Sprintf("Net Version: %v.%v", NetMajorVersion, NetMinorVersion)
}

var GNetVersion *NetVersion

func init() {
	GNetVersion = &NetVersion{}
}
