package enet

import (
	"bufio"
	"io"
	"net"
	"time"

	"go.uber.org/atomic"
)

type Connection struct {
	connId       uint64
	net          INet
	conn         *net.TCPConn
	sendBuffChan chan []byte
	exitChan     chan struct{}
	session      ISession
	state        atomic.Uint32
}

func NewConnection(connId uint64, net INet, conn *net.TCPConn, sess ISession, sendBuffMaxSize uint32) *Connection {
	ELog.Infof("[Net][Connection] ConnID=%v Bind SessID=%v", connId, sess.GetSessID())
	return &Connection{
		connId:       connId,
		net:          net,
		conn:         conn,
		session:      sess,
		sendBuffChan: make(chan []byte, sendBuffMaxSize),
		exitChan:     make(chan struct{}),
		state:        *atomic.NewUint32(ConnEstablishState),
	}
}

func (c *Connection) GetConnID() uint64 {
	return c.connId
}

func (c *Connection) writerGoroutine() {
	ELog.Infof("[Net][Connection] ConnID=%v Write Goroutine Start", c.connId)

	defer c.close(false)
	ioWriter := bufio.NewWriter(c.conn)
	var err error
	for {
		if len(c.sendBuffChan) == 0 && ioWriter.Buffered() != 0 {
			if err = ioWriter.Flush(); err != nil {
				ELog.Errorf("[Net][Connection] ConnID=%v Write Goroutine Exit Flush Error=%v", c.connId, err)
				return
			}
		}

		select {
		case datas, ok := <-c.sendBuffChan:
			if !ok {
				ELog.Errorf("[Net][Connection] Write ConnID=%v MsgBuffChan Ok Error", c.connId)
				return
			}

			ELog.Debugf("[Net][Connection] Write ConnID=%v,Len=%v", c.connId, len(datas))

			if _, err = ioWriter.Write(datas); err != nil {
				ELog.Errorf("[Net][Connection] ConnID=%v bufio Write Goroutine Exit Error=%v", c.connId, err)
				return
			}
			GSendQps.Add(1)
		case <-c.exitChan:
			{
				ELog.Infof("[Net][Connection] ConnID=%v Write Goroutine Exit", c.connId)
				return
			}
		}
	}
}

func (c *Connection) readerGoroutine() {
	ELog.Infof("[Net][Connection] ConnID=%v Read Goroutine Start", c.connId)

	defer func() {
		ELog.Infof("[Net][Connection] ConnID=%v Read Goroutine Exit", c.connId)
		c.close(false)
	}()

	for {
		if c.state.Load() != ConnEstablishState {
			return
		}

		coder := c.session.GetCoder()

		headerLen := coder.GetHeaderLen()
		headBytes := make([]byte, headerLen)
		ELog.Debugf("StartReader ConnID=%v HeaderLen=%v", c.connId, headerLen)
		if _, head_err := io.ReadFull(c.conn, headBytes); head_err != nil {
			ELog.Infof("[Net][Connection] ConnID=%v Read Goroutine Exit ReadFullError=%v", c.connId, head_err)
			return
		}

		bodyLen, bodyLenErr := coder.GetBodyLen(headBytes)
		if bodyLenErr != nil {
			ELog.Errorf("[Net][Connection] ConnID=%v Read Goroutine Exit GetUnpackBodyLenError=%V", c.connId, bodyLenErr)
			return
		}

		ELog.Debugf("StartReader ConnID=%v BodyLen=%v", c.connId, bodyLen)
		bodyBytes := make([]byte, bodyLen)
		if _, bodyErr := io.ReadFull(c.conn, bodyBytes); bodyErr != nil {
			ELog.Errorf("[Net][Connection] ConnID=%v Read Goroutine Exit ReadBodyError=%v", c.connId, bodyErr)
			return
		}

		realBodyBytes, realBodyBytesErr := coder.UnpackMsg(bodyBytes)
		if realBodyBytesErr != nil {
			ELog.Errorf("[Net][Connection] ConnID=%v Read Goroutine Exit DecodeBodyError=%v", c.connId, realBodyBytesErr)
			return
		}

		msgEvent := NewTcpEvent(ConnRecvMsgType, c, realBodyBytes)

		if c.session.GetSessionConcurrentFlag() {
			c.session.PushEvent(msgEvent)
		} else {
			c.net.PushEvent(msgEvent)
		}

		GRecvQps.Add(1)
	}
}

func (c *Connection) Start() {
	c.session.SetConnection(c)

	establishEvent := NewTcpEvent(ConnEstablishType, c, nil)
	if c.session.GetSessionConcurrentFlag() {
		c.session.StartSessionConcurrentGoroutine()
		c.session.PushEvent(establishEvent)
	} else {
		c.net.PushEvent(establishEvent)
	}

	go c.readerGoroutine()
	go c.writerGoroutine()
}

func (c *Connection) Terminate() {
	c.close(true)
}

func (c *Connection) close(terminate bool) {
	if !c.state.CAS(ConnEstablishState, ConnClosedState) {
		return
	}

	if !c.session.GetSessionConcurrentFlag() {
		closeEvent := NewTcpEvent(ConnCloseType, c, nil)
		c.net.PushEvent(closeEvent)
	}

	if terminate {
		//主动断开
		ELog.Infof("[Net][Connection] ConnID=%v Active Closed", c.connId)
		go func() {
			//等待发完所有消息或者超时后,关闭底层read,write
			closeTimer := time.NewTicker(100 * time.Millisecond)
			defer closeTimer.Stop()

			closeTimeoutTimer := time.NewTimer(60 * time.Second)
			defer closeTimeoutTimer.Stop()
			for {
				select {
				case <-closeTimer.C:
					{
						if len(c.sendBuffChan) <= 0 {
							c.onClose()
							return
						}
					}
				case <-closeTimeoutTimer.C:
					{
						c.onClose()
						return
					}
				}
			}
		}()
	} else {
		//被动断开
		ELog.Infof("[Net][Connection] ConnID=%v Passive Closed", c.connId)
		c.onClose()
	}
}

func (c *Connection) onClose() {
	if c.conn != nil {
		c.exitChan <- struct{}{} //close writer Goroutine
		c.conn.Close()           //close reader Goroutine
	}

	if c.session.GetSessionConcurrentFlag() {
		c.session.StopSessionConcurrentGoroutine()
	}
}

func (c *Connection) AsyncSend(datas []byte) {
	if c.state.Load() != ConnEstablishState {
		ELog.Warnf("[Net][Connection] ConnID=%v Send Error", c.connId)
		return
	}

	c.sendBuffChan <- datas
}

func (c *Connection) GetSession() ISession {
	return c.session
}
