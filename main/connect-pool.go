package main

import (
	"encoding/binary"
	"gate/myproto"
	"github.com/guliping-hz/mybase"
	"github.com/guliping-hz/mybase/net2"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

func PackContentToPackage(bufContent []byte) []byte {
	contentLen := len(bufContent)
	if contentLen > 65533 { //一个包最大长度不能超过65535-2
		return nil
	}
	bufPackage := make([]byte, net2.GetDefaultPackageHeadLen()+contentLen)
	binary.BigEndian.PutUint16(bufPackage, uint16(contentLen))
	copy(bufPackage[2:], bufContent)

	//fmt.Printf("%v\n", bufPackage)
	return bufPackage //返回成功消息
}

func SendToSingleServer(connect *ConnectObj, message *myproto.AgentData) bool {
	//connect := InsSinglePool.Get(false) //获取一下当前的ID
	//message.Sid = connect.Id
	buf, err := proto.Marshal(message)
	if err != nil {
		return false
	}

	buf = PackContentToPackage(buf)
	//connect := InsSinglePool.Get(true) //取出来
	ok := connect.Send(buf)
	if !ok {
		return false
	}
	return true
}

type ConnectObj struct {
	pool *ConnectPool
	net2.ClientSocket
	Id uint32
}

func (c *ConnectObj) TryReconnect() {
	for {
		err := c.ReConnect(c.pool.address)
		if err != nil {
			//mybase.W("Client OnConnect err=%s\n", err.Error())
			time.Sleep(time.Second) //1秒后尝试重新连接
			continue
		}

		mybase.I("%d connected", c.Id)
		break
	}
}

type ConnectPool struct {
	lst []*ConnectObj
	pos int

	address string

	mutex sync.Mutex
}

func (c *ConnectPool) Init(address string) {
	c.lst = make([]*ConnectObj, 0)
	c.address = address
}

func (c *ConnectPool) Create(count uint32) {
	for i := uint32(0); i < count; i++ {
		connect := new(ConnectObj)
		connect.pool = c
		connect.Id = i + 1

		peerListen := new(PeerListener)
		peerListen.connectObj = connect
		if err := connect.Connect(c.address, time.Second*10, peerListen, InsGateDD); err != nil {
			go connect.TryReconnect()
		} else {
			mybase.I("%d connect ok", connect.Id)
		}
		c.lst = append(c.lst, connect)
	}
}

func (c *ConnectPool) Get(add bool) *ConnectObj {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ret := c.lst[c.pos]
	if add {
		c.pos++
		if c.pos >= len(c.lst) {
			c.pos = 0
		}
	}
	return ret
}
