package enet

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type HttpHandlerFunc func(engine *gin.Engine) error

var GGinModeFlag uint32

func HttpListen(addr string, handler HttpHandlerFunc) {
	if atomic.CompareAndSwapUint32(&GGinModeFlag, 0, 1) {
		setGinMode()
	}

	if addr == "" {
		message := fmt.Sprintf("HttpListen Addr=%v Empty", addr)
		ELog.Errorf(message)
		time.Sleep(1 * time.Second)
		panic(message)
	}

	go func() {
		engine := gin.Default()
		handler(engine)

		ELog.Infof("Http Start Run %v", addr)
		if err := engine.Run(addr); err != nil {
			message := fmt.Sprintf("Http Run Addr=%v Error=%v", addr, err)
			ELog.Errorf(message)
			time.Sleep(1 * time.Second)
			panic(message)
		}
		ELog.Infof("Http Run %v Success", addr)

	}()
}
