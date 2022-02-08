package enet

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type ConnEvent struct {
	addr string
	sess ISession
}

type Net struct {
	recvEvtQueue IEventQueue
	connQueue    chan ConnEvent
	connState    uint32
	multiFlag    bool
}

func NewNet(maxEvtCount uint32, maxConnCount uint32) *Net {
	return &Net{
		recvEvtQueue: newEventQueue(maxEvtCount),
		connQueue:    make(chan ConnEvent, maxConnCount),
		multiFlag:    false,
	}
}

func (n *Net) Init() bool {
	ELog.Info("[Net] Init")
	return true
}

func (n *Net) UnInit() {
	if ELog != nil {
		ELog.Info("[Net] UnInit")
	}
}

func (n *Net) PushEvent(evt IEvent) {
	// if n.multiFlag {
	// 	GWorkGoroutinePool.PushEvent(evt)
	// } else {
	// 	n.recvEvtQueue.PushEvent(evt)
	// }

	n.recvEvtQueue.PushEvent(evt)
}

// func (n *Net) SetMultiProcessMsg() {
// 	n.multiFlag = true
// 	if GWorkGoroutinePool == nil {
// 		chanSize := 100000
// 		GWorkGoroutinePool = NewWorkGoroutinePool(runtime.NumCPU(), chanSize)
// 		GWorkGoroutinePool.Init()
// 	}
// }

func (n *Net) Listen(addr string, factory ISessionFactory, listenMaxCount int, sessionConcurrentFlag bool) bool {
	if addr == "" {
		return false
	}

	ELog.Infof("[Net] Start ListenTCP Addr=%v", addr)
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	if err != nil {
		message := fmt.Sprintf("[Net] Addr=%v ResolveTCPAddr Error=%v", addr, err)
		ELog.Errorf(message)
		time.Sleep(1 * time.Second)
		panic(message)
	}

	listen, listenErr := net.ListenTCP("tcp4", tcpAddr)
	if listenErr != nil {
		message := fmt.Sprintf("[Net] Addr=%v ListenTCP Error=%v", tcpAddr, listenErr)
		ELog.Errorf(message)
		time.Sleep(1 * time.Second)
		panic(message)
	}

	ELog.Infof("[Net] Addr=%v ListenTCP Success", tcpAddr)

	go func(n *Net, sessfactory ISessionFactory, listen *net.TCPListener, listenMaxCount int, sessionConcurrentFlag bool) {
		for {
			netConn, acceptErr := listen.AcceptTCP()
			if acceptErr != nil {
				ELog.Errorf("[Net] Accept Error=%v", acceptErr)
				continue
			}
			ELog.Infof("[Net] Accept Remote Addr %v", netConn.RemoteAddr().String())

			if sessfactory.GetSessionCount() >= listenMaxCount {
				ELog.Error("[Net] Conn is Full")
				netConn.Close()
				continue
			}

			session := sessfactory.CreateSession(true)
			if session == nil {
				ELog.Error("[Net] CreateSession Error")
				netConn.Close()
				continue
			}

			session.SetLocalAddr(netConn.LocalAddr().String())
			session.SetRemoteAddr(netConn.RemoteAddr().String())

			if sessionConcurrentFlag {
				session.SetSessionConcurrentFlag(true)
			}

			conn := GConnectionMgr.Create(n, netConn, session)
			go conn.Start()
		}
	}(n, factory, listen, listenMaxCount, sessionConcurrentFlag)

	return true
}

func (n *Net) Connect(addr string, sess ISession) {
	connEvt := ConnEvent{
		addr: addr,
		sess: sess,
	}
	ELog.Infof("[Net] Connect Addr=%v In ConnQueue", addr)
	n.connQueue <- connEvt

	if atomic.CompareAndSwapUint32(&n.connState, IsFreeConnectState, IsConnectingState) {
		go n.doConnectGoroutine()
	}
}

func (n *Net) doConnectGoroutine() {
	ELog.Info("[Net] doConnect Goroutine Start")
	defer func() {
		ELog.Info("[Net] doConnect Goroutine Exit")
		atomic.CompareAndSwapUint32(&n.connState, IsConnectingState, IsFreeConnectState)
	}()

	for {
		select {
		case evt := <-n.connQueue:
			addr, err := net.ResolveTCPAddr("tcp4", evt.addr)
			if err != nil {
				ELog.Errorf("[Net] Connect Addr=%v ResolveTCPAddr Error=%v", addr, err)
				continue
			}

			netConn, dial_err := net.DialTCP("tcp4", nil, addr)
			if dial_err != nil {
				ELog.Errorf("[Net] Connect Addr=%v DialTCP Error=%v", addr, dial_err)
				continue
			}

			evt.sess.SetLocalAddr(netConn.LocalAddr().String())
			conn := GConnectionMgr.Create(n, netConn, evt.sess)
			go conn.Start()
		default:
			return
		}
	}
}

func (n *Net) Run(loopCount int) bool {
	n.Update()

	i := 0
	for ; i < loopCount; i++ {
		select {
		case evt, ok := <-n.recvEvtQueue.GetEventQueue():
			if !ok {
				return false
			}
			tcpEvt := evt.(*TcpEvent)
			tcpEvt.ProcessMsg()
		default:
			return false
		}
	}
	return true
}

func (n *Net) Update() {
	if GSSSessionMgr != nil {
		GSSSessionMgr.Update()
	}

	if GCSSessionMgr != nil {
		GCSSessionMgr.Update()
	}
}

var GNet *Net

func init() {
	GNet = NewNet(NetChannelMaxSize, NetMaxConnectSize)
}
