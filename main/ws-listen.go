package main

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/guliping-hz/mybase"
	"github.com/guliping-hz/mybase/net2"
	"net/http"
	"time"
)

var InsUpgrade = websocket.Upgrader{
	// 允许跨域
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func wsHandler(ctx *gin.Context) {
	// 完成ws协议的握手操作
	// Upgrade:websocket
	conn, err := InsUpgrade.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}

	agent := net2.WebAgent(conn, websocket.BinaryMessage, time.Minute*10,
		time.Second*time.Duration(InsGateCfg.ReadSeconds), new(OnClientImp), InsGateDT)
	mybase.D("new web Client agent=%p\n", agent)
}

func startWs(r *gin.Engine) {
	//websocket
	r.GET("/ws", wsHandler) //websocket
}
