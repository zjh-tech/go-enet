package enet

import (
	"net"

	"github.com/golang/protobuf/proto"
)

type INet interface {
	PushEvent(IEvent)
	Connect(addr string, sess ISession)
	Listen(addr string, factory ISessionFactory, listenMaxCount int, sessionConcurrentFlag bool) bool
	Run(loopCount int) bool
}

type ICoder interface {
	//获取包头长度
	GetHeaderLen() uint32
	//获取包体长度
	GetBodyLen(datas []byte) (uint32, error)
	//获取包装后的数据
	PackMsg(msgId uint32, datas []byte) ([]byte, error)
	//获取原始的包体数据
	UnpackMsg(datas []byte) ([]byte, error)
	//处理消息
	ProcessMsg(datas []byte, sess ISession)
	//最大包长度
	GetPackageMaxLen() uint32
}

type ISessionOnHandler interface {
	OnHandler(msgId uint32, datas []byte)
}

type IEventQueue interface {
	PushEvent(req interface{})
	GetEventQueue() chan interface{}
}

//Tcp
type IConnection interface {
	GetConnID() uint64
	GetSession() ISession
	Start()
	AsyncSend(datas []byte)
	Terminate()
}

type IConnectionMgr interface {
	Create(net INet, conn *net.TCPConn, sess ISession) IConnection
	Remove(id uint64)
	GetConnCount() int
}

type IEvent interface {
	GetConn() IConnection
	ProcessMsg() bool
}

//ISession
type ISession interface {
	SetSessionConcurrentFlag(flag bool)

	GetSessionConcurrentFlag() bool

	StartSessionConcurrentGoroutine()

	StopSessionConcurrentGoroutine()

	PushEvent(IEvent)

	SetConnection(conn IConnection)

	GetSessID() uint64

	OnEstablish()

	OnTerminate()

	GetCoder() ICoder

	SetCoder(coder ICoder)

	GetSessionOnHandler() ISessionOnHandler

	IsListenType() bool

	IsConnectType() bool

	SetConnectType()

	SetListenType()

	SetSessionFactory(factory ISessionFactory)

	GetSessionFactory() ISessionFactory

	SendMsg(msgId uint32, datas []byte) bool

	SendProtoMsg(msgId uint32, msg proto.Message) bool

	SendJsonMsg(msgId uint32, js interface{}) bool

	Terminate()

	Update()

	GetAttach() interface{}

	SetAttach(attach interface{})

	SetLocalAddr(addr string)

	GetLocalAddr() string

	SetRemoteAddr(addr string)

	GetRemoteAddr() string
}

//ISessionFactory
type ISessionFactory interface {
	CreateSession(isListenFlag bool) ISession
	AddSession(session ISession)
	RemoveSession(id uint64)
	GetSessionCount() int
	Update()
}

//Http
type HttpCbFunc func(datas []byte) (interface{}, error)
type IHttpConnection interface {
	OnHandler(router string, datas []byte) (interface{}, error)
}

type IHttpEvent interface {
	ProcessMsg() (interface{}, error)
}
