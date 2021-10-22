package enet

import (
	"encoding/json"
	"math"
	"math/rand"

	"github.com/golang/protobuf/proto"
)

const (
	SessVerifyState uint32 = iota
	SessEstablishState
	SessCloseState
)

const (
	SSBeatHeartMaxTime   int64 = 1000 * 60 * 2
	SSBeatSendHeartTime  int64 = 1000 * 20
	SSOnceConnectMaxTime int64 = 1000 * 10
	SSMgrOutputTime      int64 = 1000 * 60
)

type VerifySessionSpec struct {
	ServerID      uint64
	ServerType    uint32
	ServerTypeStr string
	Addr          string
	Token         string
}

type RemoteSessionSpec struct {
	ServerID      uint64
	ServerType    uint32
	ServerTypeStr string
	Addr          string
}

type SSSession struct {
	Session
	sessState              uint32
	lastSendBeatHeartTime  int64
	lastCheckBeatHeartTime int64
	verifySpec             VerifySessionSpec
	remoteSpec             RemoteSessionSpec
	localToken             string
	logicServer            ILogicServer
}

func NewSSSession(isListenFlag bool) *SSSession {
	session := &SSSession{
		sessState:              SessCloseState,
		lastSendBeatHeartTime:  getMillsecond(),
		lastCheckBeatHeartTime: getMillsecond(),
	}
	session.Session.ISessionOnHandler = session
	if isListenFlag {
		session.SetListenType()
	} else {
		session.SetConnectType()
	}
	return session
}

//------------------------------------------------------------
func (s *SSSession) SetVerifySpec(verifySpec VerifySessionSpec) {
	s.verifySpec = verifySpec
}

func (s *SSSession) SetRemoteSpec(remoteSpec RemoteSessionSpec) {
	s.remoteSpec = remoteSpec
}

func (s *SSSession) SetLocalToken(token string) {
	s.localToken = token
}

func (s *SSSession) GetRemoteServerID() uint64 {
	return s.remoteSpec.ServerID
}

func (s *SSSession) GetRemoteServerType() uint32 {
	return s.remoteSpec.ServerType
}

func (s *SSSession) SetLogicServer(logicServer ILogicServer) {
	s.logicServer = logicServer
}

//---------------------------------------------------------------------
func (s *SSSession) OnEstablish() {
	s.factory.AddSession(s)
	s.sessState = SessVerifyState
	if s.IsConnectType() {
		ELog.Infof("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Establish Send Verify Req", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		req := &S2SSessionVerifyReq{
			ServerId:      s.verifySpec.ServerID,
			ServerType:    s.verifySpec.ServerType,
			ServerTypeStr: s.verifySpec.ServerTypeStr,
			Addr:          s.verifySpec.Addr,
			Token:         s.verifySpec.Token,
		}

		datas, marshalErr := json.Marshal(req)
		if marshalErr == nil {
			s.SendMsg(S2SSessionVerifyReqId, datas)
		}
		return
	}
}

func (s *SSSession) OnVerify() {
	s.sessState = SessEstablishState
	ELog.Infof("[SSSession] Remote [ID=%v,Type=%v,Ip=%v] Verify Ok", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)

	factory := s.GetSessionFactory()
	ssserverfactory := factory.(*SSSessionMgr)
	ssserverfactory.GetLogicServerFactory().SetLogicServer(s)
	s.logicServer.SetServerSession(s)
	s.logicServer.OnEstablish(s)
}

func (s *SSSession) Update() {
	now := getMillsecond()
	if (s.lastCheckBeatHeartTime + SSBeatHeartMaxTime) < now {
		ELog.Errorf("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] BeatHeart Exception", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		s.Terminate()
		return
	}

	if s.IsConnectType() {
		if (s.lastSendBeatHeartTime + SSBeatSendHeartTime) >= now {
			s.lastSendBeatHeartTime = now
			s.SendMsg(S2SSessionPingId, nil)
			ELog.Debugf("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Send Ping", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		}
	}
}

func (s *SSSession) OnTerminate() {
	if s.remoteSpec.ServerID == 0 {
		ELog.Infof("[SSSession] SessID=%v  Terminate", s.sessId)
	} else {
		ELog.Infof("[SSSession] SessID=%v [ID=%v,Type=%v,Addr=%v] Terminate", s.sessId, s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
	}
	factory := s.GetSessionFactory()
	ssserverfactory := factory.(*SSSessionMgr)
	ssserverfactory.RemoveSession(s.sessId)
	s.logicServer.SetServerSession(nil)
	s.sessState = SessCloseState

	s.logicServer.OnTerminate(s)
}

func (s *SSSession) OnHandler(msgId uint32, datas []byte) {
	if msgId == S2SSessionVerifyReqId && s.IsListenType() {
		verifyReq := &S2SSessionVerifyReq{}
		err := json.Unmarshal(datas, verifyReq)
		if err != nil {
			ELog.Infof("[SSSession] S2SSessionVerifyReq Json Unmarshal Error")
			return
		}

		var VerifyResFunc = func(errorCode uint32) {
			if errorCode == MsgFail {
				s.Terminate()
				return
			}
			s.SendMsg(S2SSessionVerifyResId, nil)
		}

		factory := s.GetSessionFactory()
		ssserverfactory := factory.(*SSSessionMgr)
		if ssserverfactory.FindSessionByServerId(verifyReq.ServerId) != nil {
			//相同的配置的ServerID服务器接入:保留旧的连接,断开新的连接
			ELog.Infof("SSSession VerifyReq ServerId=%v Already Exist", verifyReq.ServerId)
			VerifyResFunc(MsgFail)
			return
		}

		var remoteSpec RemoteSessionSpec
		remoteSpec.ServerID = verifyReq.ServerId
		remoteSpec.ServerType = verifyReq.ServerType
		remoteSpec.ServerTypeStr = verifyReq.ServerTypeStr
		remoteSpec.Addr = verifyReq.Addr
		s.SetRemoteSpec(remoteSpec)

		if verifyReq.Token != s.localToken {
			ELog.Errorf("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Recv Verify Error", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
			VerifyResFunc(MsgFail)
			return
		}

		ELog.Infof("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Recv Verify Ok", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		s.OnVerify()
		VerifyResFunc(MsgSuccess)
		return
	}

	if msgId == S2SSessionVerifyResId && s.IsConnectType() {
		ELog.Infof("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Recv Verify Ack Ok", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		s.OnVerify()
		return
	}

	if msgId == S2SSessionPingId && s.IsListenType() {
		ELog.Debugf("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Recv Ping Send Pong", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		s.lastCheckBeatHeartTime = getMillsecond()
		s.SendMsg(S2SSessionPongId, nil)
		return
	}

	if msgId == S2SSessionPongId && s.IsConnectType() {
		ELog.Debugf("[SSSession] Remote [ID=%v,Type=%v,Addr=%v] Recv Pong", s.remoteSpec.ServerID, s.remoteSpec.ServerType, s.remoteSpec.Addr)
		s.lastCheckBeatHeartTime = getMillsecond()
		return
	}

	s.lastCheckBeatHeartTime = getMillsecond()
	s.logicServer.OnHandler(msgId, datas, s)
}

//----------------------------------------------------------------------
type SSSessionCache struct {
	ServerID      uint64
	ServerType    uint32
	ServerTypeStr string
	ConnectTick   int64
}

type SSSessionMgr struct {
	nextId             uint64
	sessMap            map[uint64]ISession
	logicServerFactory ILogicServerFactory
	connectingCache    map[uint64]*SSSessionCache
	token              string
	lastUpdateTime     int64
}

func NewSSSessionMgr() *SSSessionMgr {
	return &SSSessionMgr{
		nextId:          1,
		sessMap:         make(map[uint64]ISession),
		connectingCache: make(map[uint64]*SSSessionCache),
		lastUpdateTime:  getMillsecond(),
	}
}

func (s *SSSessionMgr) Init(token string) {
	s.token = token
}

func (s *SSSessionMgr) Update() {
	now := getMillsecond()
	if (s.lastUpdateTime + SSMgrOutputTime) <= now {
		s.lastUpdateTime = now

		for _, session := range s.sessMap {
			serversess := session.(*SSSession)
			ELog.Infof("[SSSessionMgr] OutPut ServerId=%v,ServerType=%v", serversess.remoteSpec.ServerID, serversess.remoteSpec.ServerType)
		}

		for serverID, cache := range s.connectingCache {
			if cache.ConnectTick < now {
				ELog.Infof("[SSSessionMgr] Timeout Triggle  ConnectCache Del ServerId=%v,ServerType=%v", cache.ServerID, cache.ServerTypeStr)
				delete(s.connectingCache, serverID)
			}
		}
	}

	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess != nil {
			serversess.Update()
		}
	}
}

func (s *SSSessionMgr) CreateSession(isListenFlag bool) ISession {
	sess := NewSSSession(isListenFlag)
	sess.SetSessID(s.nextId)
	sess.SetCoder(NewCoder())
	sess.SetLocalToken(s.token)
	sess.SetSessionFactory(s)
	s.nextId++
	ELog.Infof("[SSSessionMgr] CreateSession SessID=%v", sess.GetSessID())
	return sess
}

func (s *SSSessionMgr) findLogicServerByServerType(serverType uint32) []ILogicServer {
	sessArray := make([]ILogicServer, 0)
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.remoteSpec.ServerType == serverType {
			sessArray = append(sessArray, serversess.logicServer)
		}
	}

	return sessArray
}

func (s *SSSessionMgr) FindSessionByServerId(serverId uint64) ISession {
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.remoteSpec.ServerID == serverId {
			return serversess
		}
	}

	return nil
}

func (s *SSSessionMgr) FindSession(id uint64) ISession {
	if id == 0 {
		return nil
	}

	if sess, ok := s.sessMap[id]; ok {
		return sess
	}
	return nil
}

func (s *SSSessionMgr) GetSessionCount() int {
	return len(s.sessMap)
}

func (s *SSSessionMgr) IsInConnectCache(serverID uint64) bool {
	_, ok := s.connectingCache[serverID]
	return ok
}

func (s *SSSessionMgr) AddSession(sess ISession) {
	s.sessMap[sess.GetSessID()] = sess
	serversess := sess.(*SSSession)
	if _, ok := s.connectingCache[serversess.GetRemoteServerID()]; ok {
		ELog.Infof("[SSSessionMgr] AddSession Triggle ConnectCache Del ServerId=%v,ServerType=%v", serversess.GetRemoteServerID(), serversess.GetRemoteServerType())

		delete(s.connectingCache, serversess.GetRemoteServerID())
	}
}

func (s *SSSessionMgr) RemoveSession(id uint64) {
	if session, ok := s.sessMap[id]; ok {
		sess := session.(*SSSession)
		if sess.remoteSpec.ServerID == 0 {
			ELog.Infof("[SSSessionMgr] Remove SessID=%v UnInit SSSession", sess.GetSessID())
		} else {
			ELog.Infof("[SSSessionMgr] Remove SessID=%v [ID=%v,Type=%v,Addr=%v] SSSession", sess.GetSessID(), sess.remoteSpec.ServerID, sess.remoteSpec.ServerType, sess.remoteSpec.Addr)
		}
		delete(s.sessMap, id)
	}
}

func (s *SSSessionMgr) SetLogicServerFactory(factory ILogicServerFactory) {
	s.logicServerFactory = factory
}

func (s *SSSessionMgr) GetLogicServerFactory() ILogicServerFactory {
	return s.logicServerFactory
}

func (s *SSSessionMgr) SendMsg(serverId uint64, msgId uint32, datas []byte) {
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.remoteSpec.ServerID == serverId {
			serversess.SendMsg(msgId, datas)
			return
		}
	}
}

func (s *SSSessionMgr) SendProtoMsgByServerId(serverId uint64, msgId uint32, msg proto.Message) bool {
	if serverId == 0 {
		return false
	}

	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.remoteSpec.ServerID == serverId {
			serversess.SendProtoMsg(msgId, msg)
			return true
		}
	}

	return false
}

func (s *SSSessionMgr) SendProtoMsgBySessionID(sessionID uint64, msgId uint32, msg proto.Message) bool {
	serversess, ok := s.sessMap[sessionID]
	if ok {
		return serversess.SendProtoMsg(msgId, msg)
	}

	return false
}

func (s *SSSessionMgr) SendJsonMsgBySessionID(sessionID uint64, msgId uint32, js interface{}) bool {
	serversess, ok := s.sessMap[sessionID]
	if ok {
		return serversess.SendJsonMsg(msgId, js)
	}

	return false
}

func (s *SSSessionMgr) BroadcastProtoMsg(serverType uint32, msgId uint32, msg proto.Message) {
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.GetRemoteServerType() == serverType {
			serversess.SendProtoMsg(msgId, msg)
		}
	}
}

func (s *SSSessionMgr) BroadcastJsonMsg(serverType uint32, msgId uint32, js interface{}) {
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.GetRemoteServerType() == serverType {
			serversess.SendJsonMsg(msgId, js)
		}
	}
}

func (s *SSSessionMgr) BroadcastMsg(serverType uint32, msgId uint32, datas []byte) {
	for _, session := range s.sessMap {
		serversess := session.(*SSSession)
		if serversess.GetRemoteServerType() == serverType {
			serversess.SendMsg(msgId, datas)
		}
	}
}

func (s *SSSessionMgr) SendProtoMsgByRandom(serverType uint32, msgId uint32, msg proto.Message) bool {
	logicServerArray := s.findLogicServerByServerType(serverType)
	logicServerLen := uint64(len(logicServerArray))
	if logicServerLen == 0 {
		ELog.Warnf("ServerType=%v,MsgId=%v,Msg=%v SendProtoMsgByRandom Len=0", serverType, msgId, msg)
		return false
	}

	logicServerIndex := rand.Intn(int(logicServerLen))
	return logicServerArray[logicServerIndex].GetServerSession().SendProtoMsg(msgId, msg)
}

func (s *SSSessionMgr) SendJsonMsgByRandom(serverType uint32, msgId uint32, js interface{}) bool {
	logicServerArray := s.findLogicServerByServerType(serverType)
	logicServerLen := uint64(len(logicServerArray))
	if logicServerLen == 0 {
		ELog.Warnf("ServerType=%v,MsgId=%v,Msg=%v SendJsonMsgByRandom Len=0", serverType, msgId, js)
		return false
	}

	logicServerIndex := rand.Intn(int(logicServerLen))
	return logicServerArray[logicServerIndex].GetServerSession().SendJsonMsg(msgId, js)
}

func (s *SSSessionMgr) SSServerConnect(verifySpec VerifySessionSpec, remoteSepc RemoteSessionSpec) {
	session := s.CreateSession(false)
	session.SetRemoteAddr(remoteSepc.Addr)
	cache := &SSSessionCache{
		ServerID:      remoteSepc.ServerID,
		ServerType:    remoteSepc.ServerType,
		ServerTypeStr: remoteSepc.ServerTypeStr,
		ConnectTick:   getMillsecond() + SSOnceConnectMaxTime,
	}
	s.connectingCache[remoteSepc.ServerID] = cache
	ELog.Infof("[SSSessionMgr]ConnectCache Add ServerId=%v,ServerType=%v", remoteSepc.ServerID, remoteSepc.ServerTypeStr)

	serverSession := session.(*SSSession)
	serverSession.SetVerifySpec(verifySpec)
	serverSession.SetRemoteSpec(remoteSepc)
	GNet.Connect(remoteSepc.Addr, serverSession)
}

func (s *SSSessionMgr) SSServerListen(addr string) bool {
	return GNet.Listen(addr, s, math.MaxInt32, false)
}

var GSSSessionMgr *SSSessionMgr

func init() {
	GSSSessionMgr = NewSSSessionMgr()
}
