package enet

import "runtime"

//C++ 避免创建Goroutine来回创建
//Go  每个Session有个Channel,每个Entity有个Channel,并发的数量可以动态(建议使用这种模式)

type WorkGoroutinePool struct {
	poolSize      int
	eventChanSets []chan IEvent
}

func NewWorkGoroutinePool(poolSize, chanSize int) *WorkGoroutinePool {
	cpu_num := runtime.NumCPU()
	if poolSize <= cpu_num {
		poolSize = cpu_num
	}

	chan_min_size := 100000
	if chanSize <= chan_min_size {
		chanSize = chan_min_size
	}

	obj := &WorkGoroutinePool{
		poolSize:      poolSize,
		eventChanSets: make([]chan IEvent, 0),
	}

	for i := 0; i < poolSize; i++ {
		event_chan := make(chan IEvent, chanSize)
		obj.eventChanSets = append(obj.eventChanSets, event_chan)
	}

	return obj
}

func (a *WorkGoroutinePool) Init() bool {
	for i := 0; i < a.poolSize; i++ {
		go a.run(i)
	}

	return true
}

func (a *WorkGoroutinePool) run(index int) {
	evtQueue := a.eventChanSets[index]
	for {
		select {
		case evt := <-evtQueue:
			{
				DoMsgHandler(evt)
			}
		}
	}
}

func (a *WorkGoroutinePool) PushEvent(evt IEvent) bool {
	if evt == nil {
		return false
	}

	conn := evt.GetConn()
	if conn == nil {
		return false
	}

	sess := conn.GetSession()
	if sess == nil {
		return false
	}

	workerId := int(sess.GetSessID()) % a.poolSize
	a.eventChanSets[workerId] <- evt
	return true
}

func DoMsgHandler(evt IEvent) {
	if evt == nil {
		return
	}

	evt.ProcessMsg()
}

var GWorkGoroutinePool *WorkGoroutinePool
