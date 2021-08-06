package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"time"
)

type TcpSession struct {
	Conn   net.Conn
	Online bool
}

func main() {
	var cPort *int
	cPort = flag.Int("cp", 8080, "监听端口")
	var sAddrStr *string
	sAddrStr = flag.String("s", "127.0.0.1:8081", "服务器地址")
	flag.Parse()
	fmt.Println("get server [", *sAddrStr, "]")

	udpServerAddr, err := net.ResolveUDPAddr("udp", *sAddrStr)
	tcpServerAddr, _ := net.ResolveTCPAddr("tcp", *sAddrStr)
	if err != nil {
		fmt.Println("parse server location error")
		return
	}
	if *cPort < 1 || *cPort > 65535 {
		fmt.Println("illegal port ", *cPort)
		return
	}

	go UdpServiceInit(udpServerAddr, cPort)
	go TcpServiceInit(tcpServerAddr, cPort)
	for {
		time.Sleep(time.Second * 5)
	}
}

func UdpServiceInit(udpServerAddr *net.UDPAddr, port *int) {
	//启动UDP服务器
	udpAddr := net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: *port,
	}
	fmt.Println("starting udp server at [", udpAddr.String(), "]...")
	udpListener, err := net.ListenUDP("udp", &udpAddr)
	for err != nil || udpListener == nil {
		if err != nil {
			fmt.Println("start udp server error", err.Error())
		}
		fmt.Println("restarting udp server...")
		udpListener, err = net.ListenUDP("udp", &udpAddr)
	}
	defer func(udpListener *net.UDPConn) {
		_ = udpListener.Close()
	}(udpListener)
	//监听并转发UDP数据
	buffer := make([]byte, 2048)
	for {
		dataLen, addr, err := udpListener.ReadFromUDP(buffer[:])
		if err != nil {
			fmt.Println("read udp data error\n", err.Error())
			continue
		}
		fmt.Println(addr.String(), " len=", dataLen)

		go UdpDataForward(udpServerAddr, addr, buffer, dataLen)
		buffer = make([]byte, 2048)
	}
}

func UdpDataForward(serverAddr, clientAddr *net.UDPAddr, dataToSend []byte, dataLen int) {
	udpConn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		fmt.Println("dial to udp server error\n", err.Error())
		return
	}

	_, err = udpConn.Write(dataToSend[:dataLen])
	if err != nil {
		fmt.Println("send udp data to server error\n", err.Error())
		return
	}

	fmt.Println(udpConn.LocalAddr())
	fmt.Println("fuck")
	buffer := make([]byte, 2048)
	addr, _ := net.ResolveUDPAddr("udp", udpConn.LocalAddr().String())
	udpServerListener, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("udp listen to [", udpConn.LocalAddr().String(), "] error")
		return
	}
	for {
		dataLen, addr, err := udpServerListener.ReadFromUDP(buffer[:])
		if err != nil {
			fmt.Println("read udp data error\n", err.Error())
			continue
		}
		if dataLen == 0 {
			continue
		}
		fmt.Println(addr.String(), " len=", dataLen)
		//发送数据回客户端
		udpConn, err := net.DialUDP("udp", nil, clientAddr)
		if err != nil {
			fmt.Println("dial to udp client error\n", err.Error())
			return
		}
		fmt.Println("send data to [", udpConn.RemoteAddr(), "]")
		_, err = udpConn.Write(buffer[:dataLen])
		if err != nil {
			fmt.Println("send udp data to client error\n", err.Error())
			return
		}
		break
	}
}

func TcpServiceInit(tcpServerAddr *net.TCPAddr, port *int) {
	//启动TCP服务器
	tcpAddr := net.TCPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: *port,
	}
	fmt.Println("starting tcp server at [", tcpAddr.String(), "]...")
	tcpListener, err := net.ListenTCP("tcp", &tcpAddr)
	for err != nil || tcpListener == nil {
		if err != nil {
			fmt.Println("start tcp server error", err.Error())
		}
		fmt.Println("restarting tcp server...")
		tcpListener, err = net.ListenTCP("tcp", &tcpAddr)
	}
	fmt.Println("tcp server waiting for client...")
	//TCP服务端监听服务
	for {
		//等待TCP客户端
		tcpClientConn, connErr := tcpListener.Accept()
		if connErr != nil {
			fmt.Println("tcp error accepting", connErr.Error())
			//跳过本次连接建立，连接建立失败
			fmt.Println("cannot connect to tcp client, close current session...")
			continue
		}
		fmt.Println("connected to tcp client[", tcpClientConn.RemoteAddr(), "]")
		//连接TCP服务器并创建连接
		fmt.Println("connecting to tcp server[ ", tcpServerAddr.String(), " ]...")
		tcpServerConn, tcpServerErr := net.DialTimeout("tcp", tcpServerAddr.String(), time.Second*3)
		if tcpServerErr != nil {
			//无法连接至服务器时直接断开连接
			err := tcpClientConn.Close()
			if err != nil {
				fmt.Println("close conn error")
				fmt.Println(err.Error())
			}
			//跳过本次连接建立，连接建立失败
			fmt.Println("cannot connect to tcp server, close current session...")
			continue
		}
		//建立双向转发
		tcpClient := &TcpSession{
			Conn:   tcpClientConn,
			Online: true,
		}
		tcpServer := &TcpSession{
			Conn:   tcpServerConn,
			Online: true,
		}
		go TcpDataForward(tcpClient, tcpServer)
		go TcpDataForward(tcpServer, tcpClient)
	}
}

func TcpDataForward(in, out *TcpSession) {
	reader := bufio.NewReader(in.Conn)
	buffer := make([]byte, 2048)
	for {
		//接收
		bufLen, err := reader.Read(buffer)
		if err != nil {
			switch err {
			case io.EOF:
				fmt.Println("tcp session [", in.Conn.RemoteAddr(), "->", out.Conn.RemoteAddr(), "] closed(EOF)")
				break
			default:
				fmt.Println("read msg from [ ", in.Conn.RemoteAddr(), " ] error, close current session...")
				fmt.Println(err.Error())
				break
			}
			closeSession(out)
			closeSession(in)
			break
		}
		//发送
		if bufLen != 0 {
			buffer = buffer[:bufLen]
			_, err := out.Conn.Write(buffer)
			buffer = make([]byte, 2048)
			if err != nil {
				fmt.Println("send msg to [ ", out.Conn.RemoteAddr(), " ] error, close current session...")
				fmt.Println(err.Error())
				closeSession(out)
				closeSession(in)
				break
			}
		}
	}
}

func closeSession(conn *TcpSession) {
	if !(*conn).Online {
		return
	}
	err := (*conn).Conn.Close()
	(*conn).Online = false
	if err != nil {
		if _, ok := err.(*net.OpError); ok {
			fmt.Println("conn closed [", (*conn).Conn.RemoteAddr(), "]")
			return
		}
		fmt.Println("close conn error")
		fmt.Println(err.Error())
		panic(err)
	}
}
