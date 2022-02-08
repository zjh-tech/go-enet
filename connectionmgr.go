package enet

import (
	"net"
	"sync"
)

type ConnectionMgr struct {
	conns      map[uint64]IConnection
	connLocker sync.RWMutex
	nextId     uint64
}

func NewConnectionMgr() *ConnectionMgr {
	return &ConnectionMgr{
		conns:  make(map[uint64]IConnection),
		nextId: 0,
	}
}

func (c *ConnectionMgr) Create(net INet, netConn *net.TCPConn, sess ISession, sendBuffMaxSize uint32) IConnection {
	netConn.SetNoDelay(false)
	c.connLocker.Lock()
	defer c.connLocker.Unlock()
	c.nextId++
	conn := NewConnection(c.nextId, net, netConn, sess, sendBuffMaxSize)
	c.conns[conn.GetConnID()] = conn
	ELog.Infof("[Net][ConnectionMgr] Add ConnID=%v Connection", conn.connId)
	return conn
}

func (c *ConnectionMgr) Remove(id uint64) {
	c.connLocker.Lock()
	defer c.connLocker.Unlock()

	delete(c.conns, id)
	ELog.Infof("[Net][ConnectionMgr] Remove ConnID=%v Connection", id)
}

func (c *ConnectionMgr) GetConnCount() int {
	c.connLocker.Lock()
	defer c.connLocker.Unlock()

	return len(c.conns)
}

var GConnectionMgr *ConnectionMgr

func init() {
	GConnectionMgr = NewConnectionMgr()
}
