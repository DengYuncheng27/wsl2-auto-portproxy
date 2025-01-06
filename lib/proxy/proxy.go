package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
)

type Proxy struct {
	Type      string
	Port      int64
	ProxyPort int64
	Listener  *net.TCPListener
	IsRunning bool
	WslIp     string
}

func (p *Proxy) Start() error {
	localAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", p.ProxyPort))
	if err != nil {
		log.Printf("resove local Addr error,%s\n", err)
		return err
	}
	p.Listener, err = net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Printf("Could not start proxy server on %d: %v\n", p.Port, err)
		return err
	}
	log.Printf("new proxy start in port:%d->%d", p.ProxyPort, p.Port)
	go func() {
		for {
			conn, err := p.Listener.AcceptTCP()
			if err != nil {
				log.Println("Could not accept client connection:", err)
				break
			}
			go p.handleTCPConn(conn, 5000)
		}
	}()
	p.IsRunning = true
	return nil
}

func (p *Proxy) Stop() error {
	p.IsRunning = false
	log.Printf("proxy stop, port:%d->%d", p.ProxyPort, p.Port)
	return p.Listener.Close()
}

func (p *Proxy) handleTCPConn(conn *net.TCPConn, timeout int64) {
	log.Printf("Client '%v' connected!\n", conn.RemoteAddr())

	// 获取客户端 IP
	clientIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

	// 检查端口是否有 IP 白名单限制
	conf, _ := config.GetConfig()
	if ipList, exists := conf.PortIpWhite[fmt.Sprintf("%d", p.Port)]; exists {
		isAllowed := false
		for _, ip := range ipList {
			if clientIP == ip {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			log.Printf("Unauthorized access attempt to port %d from IP: %s\n", p.Port, clientIP)
			conn.Close()
			return
		}
		log.Printf("Authorized access to port %d from IP: %s\n", p.Port, clientIP)
	}

	_ = conn.SetKeepAlive(true)
	_ = conn.SetKeepAlivePeriod(time.Second * 15)
	targetAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", p.WslIp, p.Port))
	if err != nil {
		log.Printf("resove remote Addr error,%s\n", err)
	}
	c, err := net.DialTimeout("tcp", targetAddr.String(), time.Duration(timeout)*time.Second)
	client, _ := c.(*net.TCPConn)
	if err != nil {
		log.Println("Could not connect to remote server:", err)
		return
	}
	defer client.Close()
	defer conn.Close()
	log.Printf("Connection to server '%v' established!\n", client.RemoteAddr())

	_ = client.SetKeepAlive(true)
	_ = client.SetKeepAlivePeriod(time.Second * 15)

	stop := make(chan bool)

	go func() {
		_, err := io.Copy(client, conn)
		if err != nil {
			log.Println(err)
		}
		stop <- true
	}()

	go func() {
		_, err := io.Copy(conn, client)
		if err != nil {
			log.Println(err)
		}
		stop <- true
	}()

	<-stop
}
