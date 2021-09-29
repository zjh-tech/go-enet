package enet

//Tcp
type TcpEvent struct {
	eventType uint32
	conn      IConnection
	datas     interface{}
}

func NewTcpEvent(t uint32, c IConnection, datas interface{}) *TcpEvent {
	return &TcpEvent{
		eventType: t,
		conn:      c,
		datas:     datas,
	}
}

func (t *TcpEvent) GetConn() IConnection {
	return t.conn
}

func (t *TcpEvent) ProcessMsg() bool {
	if t.conn == nil {
		ELog.Error("[Net] Run Conn Is Nil")
		return false
	}

	session := t.conn.GetSession()
	if session == nil {
		ELog.Error("[Net] Run Session Is Nil")
		return false
	}

	if t.eventType == ConnEstablishType {
		//session.SetConnection(t.conn)
		session.OnEstablish()
	} else if t.eventType == ConnRecvMsgType {
		datas := t.datas.([]byte)
		session.GetCoder().ProcessMsg(datas, session)
	} else if t.eventType == ConnCloseType {
		session.SetConnection(nil)
		GConnectionMgr.Remove(t.conn.GetConnID())
		session.OnTerminate()
	}
	return true
}

//Http
type HttpEvent struct {
	httpConn IHttpConnection
	router   string
	datas    []byte
	paras    interface{}
	cbFunc   HttpCbFunc
}

func NewHttpEvent(httpConn IHttpConnection, router string, cbFunc HttpCbFunc, datas []byte, paras interface{}) *HttpEvent {
	return &HttpEvent{
		httpConn: httpConn,
		router:   router,
		datas:    datas,
		paras:    paras,
		cbFunc:   cbFunc,
	}
}

func (h *HttpEvent) ProcessMsg() bool {
	if h.httpConn == nil {
		ELog.Error("[Net] ProcessMsg Run HttpConnection Is Nil")
		return false
	}

	if h.cbFunc != nil {
		h.cbFunc(h.datas, h.paras)
	} else {
		h.httpConn.OnHandler(h.router, h.datas, h.paras)
	}

	return true
}
