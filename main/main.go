package main

import (
	"context"
	"flag"
	"fmt"
	"gate/myproto"
	"github.com/gin-gonic/gin"
	"github.com/guliping-hz/mybase"
	"github.com/guliping-hz/mybase/net2"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/titanous/json5"
	"google.golang.org/protobuf/proto"
	"log"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go mod download github.com/sirupsen/logrus

type GateCfg struct {
	ListenPort      int    `json:"listen_port"`      //监听端口
	Address         string `json:"address"`          //指定代理的地址  127.0.0.1:3306
	ReadSeconds     int    `json:"read_seconds"`     //读取超时时间 秒
	ListenWebsocket bool   `json:"listen_websocket"` //是否启动websocket监听   域名/ws
	IsSingle        bool   `json:"is_single"`        //是否只用有限连接目标地址
	SingleUnionId   uint32 `json:"single_union_id"`  //走指定连接数的时候，网关的唯一ID。确保多个网关ID不冲突 127.0.0.1:5003
	ConnectCnt      uint32 `json:"connect_cnt"`      //需要创建的连接数
	WsGate          string `json:"ws_gate"`
	WsWeight        int32  `json:"ws_weight"`

	//`json:"omitempty"` 指定空的时候不序列化
	Server      *net2.ServerSocket `json:"-"` //指定不序列化
	Client2Peer sync.Map           `json:"-"` //Client->peer
	Peer2Client sync.Map           `json:"-"` //peer->Client

	Id2Client sync.Map `json:"-"`
}

var (
	InsGateCfg    = new(GateCfg)
	InsSinglePool *ConnectPool

	InsGateDD = new(net2.DataDecodeBinaryBigEnd)
	InsGateDT = new(net2.DataDecodeText)
)

// 用于连接目标地址ip端口
type PeerListener struct {
	connectObj *ConnectObj
}

var InsPeer = new(PeerListener)

/*
*
连接上服务器回调,或者服务器accept某个客户端连接
*/
func (p *PeerListener) OnConnect(peer net2.Conn) {
	mybase.D("peer OnConnect Client=%v\n", peer.RemoteAddr())

	if InsGateCfg.IsSingle {
		SendToSingleServer(&myproto.AgentData{
			Id: InsGateCfg.SingleUnionId,
			//Sid:    p.connectObj.Id,
			Status: myproto.Status_Init,
			Ws:     InsGateCfg.WsGate,
			Weight: InsGateCfg.WsWeight,
		})
	}
}

/*
*
只要我们曾经连接上服务器过，OnClose必定会回调。代表一个当前的socket已经关闭
*/
func (p *PeerListener) OnClose(peer net2.Conn, byLocalNotRemote bool) {
	mybase.D("peer OnClose\n")

	if InsGateCfg.IsSingle {
		go p.connectObj.TryReconnect()
	} else {
		clientI, ok := InsGateCfg.Peer2Client.LoadAndDelete(peer.SessionId())
		if !ok {
			return
		}
		client, ok := clientI.(net2.Conn)
		if !ok {
			return
		}
		client.SafeClose(byLocalNotRemote)
	}
}

// 连接超时,写入超时,读取超时回调
func (p *PeerListener) OnTimeout(net2.Conn) {
	log.Printf("peer OnTimeout\n")
}

// 网络错误回调，之后直接close
func (p *PeerListener) OnNetErr(net2.Conn) {

}

/*
接受到信息
@return 返回true表示可以继续热恋，false表示要分手了。
*/
func (p *PeerListener) OnRecvMsg(peer net2.Conn, buf []byte) bool {
	//log.Printf("peer OnRecvMsg buf[%d]%v\n", len(buf), buf)

	var clientI any
	var ok bool
	if InsGateCfg.IsSingle {
		ad := new(myproto.AgentData)
		if err := proto.Unmarshal(buf[InsGateDD.GetPackageHeadLen():], ad); err != nil {
			mybase.W("Unmarshal AgentData,e=%s", err.Error())
			return false //终止连接
		}

		//mybase.I("recv data=%+v", ad)
		if ad.Close { //服务器通知关闭客户端连接。
			if v, ok := InsGateCfg.Id2Client.LoadAndDelete(ad.CliId); ok {
				if client, ok := v.(net2.Conn); ok {
					client.SafeClose(true)
				}
			}
			return true
		}

		buf = ad.Data //更新buff
		clientI, ok = InsGateCfg.Id2Client.Load(ad.CliId)
	} else {
		clientI, ok = InsGateCfg.Peer2Client.Load(peer.SessionId())
	}

	if !ok {
		mybase.W("can't find the Client,peer=%v\n", &peer)
		return InsGateCfg.IsSingle //单连接模式不能关闭
	}

	client, ok := clientI.(net2.Conn)
	if !ok {
		mybase.W("can't change to Client,peer=%v,clientI=%v\n", &peer, clientI)
		return InsGateCfg.IsSingle //单连接模式不能关闭
	}
	client.Send(buf)

	return true
}

// 监听客户端发上来的连接
type OnClientImp struct {
}

/*
*
连接上服务器回调
*/
func (o *OnClientImp) OnConnect(conn net2.Conn) {
	//mybase.I("cli:%d OnConnect from=%v\n", conn.SessionId(), conn.RemoteAddr())
	log.Println("cli OnConnect from=", conn.SessionId())

	if InsGateCfg.IsSingle {
		//mybase.I("new connected %d", conn.SessionId())
		InsGateCfg.Id2Client.Store(conn.SessionId(), conn)
	} else {
		peer := new(net2.ClientSocket)
		err := peer.Connect(InsGateCfg.Address, time.Second*10, InsPeer, InsGateDT)
		if err != nil {
			mybase.W("Client OnConnect err=%s\n", err.Error())
			conn.SafeClose(false)
			return
		}

		//保存对应的实例
		InsGateCfg.Client2Peer.Store(conn.SessionId(), peer)
		mybase.D("OnConnect peer=%d\n", peer.SessionId())
		InsGateCfg.Peer2Client.Store(peer.SessionId(), conn)

		//尝试获取一下。 only for debug
		_, ok := InsGateCfg.Client2Peer.Load(conn.SessionId())
		if !ok {
			mybase.W("can't find the peer,cli=%d\n", conn.SessionId())
		}
		_, ok = InsGateCfg.Peer2Client.Load(peer.SessionId())
		if !ok {
			mybase.W("can't find the Client,peer=%d\n", peer.SessionId())
		}
	}
}

/*
*
只要我们曾经连接上服务器过，OnClose必定会回调。代表一个当前的socket已经关闭
*/
func (o *OnClientImp) OnClose(conn net2.Conn, byLocalNotRemote bool) {
	//mybase.I("cli:%d OnClose\n", conn.SessionId())

	if InsGateCfg.IsSingle {
		go func() {
			SendToSingleServer(&myproto.AgentData{
				Id:     InsGateCfg.SingleUnionId,
				CliId:  conn.SessionId(),
				Status: myproto.Status_Close,
			})
		}()
		//移除即可
		InsGateCfg.Id2Client.Delete(conn.SessionId())
	} else {
		peerI, ok := InsGateCfg.Client2Peer.LoadAndDelete(conn.SessionId())
		if !ok {
			mybase.W("can't find the pear,cli=%d\n", conn.SessionId())
			return
		}
		peer := peerI.(*net2.ClientSocket)
		peer.SafeClose(byLocalNotRemote) //safe close 会触发 peer.OnClose
	}
}

/*
*
连接超时,写入超时,读取超时回调 之后直接close
*/
func (o *OnClientImp) OnTimeout(conn net2.Conn) {
	mybase.D("cli:%d OnTimeout\n", conn.SessionId())
}

/*
*
网络错误回调，之后直接close
*/
func (o *OnClientImp) OnNetErr(conn net2.Conn) {
	mybase.D("cli:%d OnNetErr err=%s\n", conn.SessionId(), conn.Error())
}

/*
*
接受到信息
@return 返回true表示可以继续热恋，false表示要分手了。
*/
func (o *OnClientImp) OnRecvMsg(conn net2.Conn, buf []byte) bool {
	mybase.D("Client OnRecvMsg buf[%d]\n", len(buf))

	if InsGateCfg.IsSingle {
		if _, ok := InsGateCfg.Id2Client.Load(conn.SessionId()); !ok {
			return false
		}

		if ok := SendToSingleServer(&myproto.AgentData{
			Id:     InsGateCfg.SingleUnionId,
			CliId:  conn.SessionId(),
			Status: myproto.Status_Live,
			Data:   buf,
		}); !ok {
			return false
		}
	} else {
		peerI, ok := InsGateCfg.Client2Peer.Load(conn.SessionId())
		if !ok {
			mybase.W("can't find the pear, cli=%d\n", conn.SessionId())
			return false
		}
		peer, ok := peerI.(*net2.ClientSocket)
		if !ok {
			mybase.W("can't change to peer, cli=%d\n", conn.SessionId())
			return false
		}
		peer.Send(buf)
	}

	return true
}

// 服务完成开始监听客户端连接
func (o *OnClientImp) OnServerListen() {
	//util.I("OnServerListen o == ssb:%v", InsGateCfg.Server == server)
}
func (o *OnClientImp) OnServerErr(se net2.StackError) {
	builder := strings.Builder{}
	builder.Write(se.Stack())
	log.Printf("OnServerErr err=%s,stack=%s\n", se.Error(), builder.String())
}

// 这里OnServerClose始终会回调
func (o *OnClientImp) OnServerClose() {
	log.Printf("OnServerClose\n")
}

func main() {
	defer func() {
		time.Sleep(time.Millisecond * 100)

		//now := time.Now()     //获取当前时间
		//exeName := os.Args[0] //获取程序名称
		//
		//if err := recover(); err != nil {
		//	time_str := now.Format("20060102150405")                  //设定时间格式
		//	fname := fmt.Sprintf("%s-%s-dump.log", exeName, time_str) //保存错误信息文件名:程序名-进程ID-当前时间（年月日时分秒）
		//
		//	f, err1 := os.Create(fname)
		//	if err1 == nil {
		//		defer f.Close()
		//		builder := &strings.Builder{}
		//		builder.Write(debug.Stack())
		//		log.Println("dump to file err=", err, "\nstack=\n", builder.String())
		//		_, _ = f.WriteString(fmt.Sprintf("%v\r\n", err)) //输出panic信息
		//		_, _ = f.WriteString("========\r\n")
		//		_, _ = f.WriteString(string(debug.Stack())) //输出堆栈信息
		//	}
		//}
	}()

	isProduct := flag.String("env", "production", "debug/production")
	flag.Parse()

	inProduct := *isProduct == "production"
	err := mybase.InitLogModule(".", "gate-main", 60, inProduct, logrus.TraceLevel, context.Background())
	if err != nil {
		log.Println("new log error. err=", err)
		return
	}

	filePath, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		mybase.E("path err=", err.Error())
		return
	}
	fullPathFile := filePath + "/gate.json5"
	buf, err := os.ReadFile(fullPathFile)
	if err != nil {
		mybase.W("load conf ReadFile[%s] fail=%s", fullPathFile, err.Error())
	} else {
		err = json5.Unmarshal(buf, InsGateCfg)
	}

	if err != nil {
		mybase.W("err=%s\n", err.Error())

		path, _ := filepath.Abs(filepath.Dir(os.Args[0]))
		path += "/gate.json5"
		f, err := os.Create(path)
		if err == nil {
			f.WriteString(`{
  //代理监听端口
  listen_port: 9999,
  //代理tcp地址
  address: "127.0.0.1:3306",
  //读超时等待
  read_seconds: 60,
  //是否监听websocket
  listen_websocket: true,
  //是否只用有限连接数=连接目标地址
  is_single: true,
  //走指定连接数的时候，网关的唯一ID。确保多个网关ID不冲突。
  single_union_id: "1",
  //需要创建的连接数
  connect_cnt: 10,
  //对外开放地址 ws://127.0.0.1:9999/ws，或者是配置在nginx中的代理地址，如果与默认的服务器ws地址一样可不填，如果配置在新的一台服务器，需配置新地址
  ws_gate: "",
  //ws权重信息
  ws_weight: 0,
}`)
			f.Sync()
			f.Close()

			//重新读取下配置
			buf, err := os.ReadFile(fullPathFile)
			if err != nil {
				mybase.E("load conf ReadFile[%s] fail=%s", fullPathFile, err.Error())
				return
			}
			err = json5.Unmarshal(buf, InsGateCfg)
			if err != nil {
				mybase.E("load conf ReadFile[%s] fail=%s", fullPathFile, err.Error())
				return
			}
		} else {
			mybase.E("gate.json5 create fail=%s", err.Error())
		}
	}

	mybase.I("listenPort=%d => address=%s\n", InsGateCfg.ListenPort, InsGateCfg.Address)

	c := cron.New() //cron.WithSeconds()
	// “秒 分 时 日 月 周”
	_, _ = c.AddFunc("0 0 * * *", mybase.CheckDay) //"CheckDay" 每天更换一个日志文件
	c.Start()

	if InsGateCfg.IsSingle {
		InsSinglePool = new(ConnectPool)
		InsSinglePool.Init(InsGateCfg.Address)
		InsSinglePool.Create(InsGateCfg.ConnectCnt)
	}

	if InsGateCfg.ListenWebsocket {
		if inProduct {
			gin.SetMode(gin.ReleaseMode)
		}
		r := gin.New()
		r.Use(gin.Recovery())

		//支持跨域
		r.Use(mybase.CrossMidW)
		startWs(r)
		go func() {
			if err = r.Run(fmt.Sprintf(":%d", InsGateCfg.ListenPort)); err != nil {
				log.Printf("err=%s\n", err.Error())
			}
		}()
	} else {
		InsGateCfg.Server = net2.NewServer(fmt.Sprintf("0.0.0.0:%d", uint16(InsGateCfg.ListenPort)), time.Minute*10,
			time.Second*time.Duration(InsGateCfg.ReadSeconds), new(OnClientImp), InsGateDT)
		err = InsGateCfg.Server.Listen()
		if err != nil {
			log.Printf("err=%s\n", err.Error())
			return
		}
	}

	//go func() {
	//	//http://127.0.0.1:7001/debug/pprof/
	//	mybase.I("http debug port 7002")
	//	err = http.ListenAndServe(":7002", nil)
	//	if err != nil {
	//		log.Printf("ListenAndServe: %v", err)
	//	}
	//}()

	//func() {
	//	for {
	//		//checkPid := os.Getpid() // process.test
	//		//ret, _ := process.NewProcess(int32(checkPid))
	//		//mem, _ := ret.MemoryInfo()
	//		//io, _ := ret.IOCounters()
	//		//fds, _ := ret.NumFDs()
	//		//ths, _ := ret.NumThreads()
	//		//mybase.I("main loop MemoryInfo=%s,io=%s,描述符=%d,线程=%d", mem, io, fds, ths)
	//
	//		//type profile struct {
	//		//	Name  string
	//		//	Count int
	//		//}
	//		//var profiles []profile
	//		//for _, p := range pprof.Profiles() {
	//		//	profiles = append(profiles, profile{
	//		//		Name:  p.Name(),
	//		//		Count: p.Count(),
	//		//	})
	//		//}
	//
	//		//检测核心数据是否有泄漏
	//		//mybase.I("main loop info=%+v", profiles)
	//		//runtime.GC()
	//		time.Sleep(time.Second * 25)
	//	}
	//}()

	for {
		time.Sleep(time.Second)
	}
}
