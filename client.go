package p2p

import (
	"fmt"
	"net"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type Peer struct {
	Id           string
	PublicAddr   *net.UDPAddr
	IntranetAddr *net.UDPAddr
}

func (p *Peer) String() string {
	if p.IntranetAddr == nil {
		p.IntranetAddr = &net.UDPAddr{}
	}
	return fmt.Sprintf("[%s]-[%s]-[%s]", p.Id, p.PublicAddr.String(), p.IntranetAddr.String())
}

type request struct {
	Type byte
	Data []byte
	Src  *net.UDPAddr
	Dst  *net.UDPAddr
}

type Client struct {
	Local     *Peer
	Server    *Peer
	PeerMap   map[string]*Peer
	IpToId    map[string]string
	ServerReq chan *request
	PeerReq   chan *request
	ln        *net.UDPConn
	interval  time.Duration
}

func NewClient(id, serverIp string) *Client {
	serverAddr, err := net.ResolveUDPAddr("udp", serverIp)
	if err != nil {
		log.Panic(err)
	}
	c := &Client{
		Local: &Peer{
			Id:           id,
			IntranetAddr: &net.UDPAddr{IP: net.IPv4zero, Port: 0},
		},
		Server: &Peer{
			Id:         "server",
			PublicAddr: serverAddr,
		},
		PeerMap:   make(map[string]*Peer),
		IpToId:    make(map[string]string),
		ServerReq: make(chan *request, 128),
		PeerReq:   make(chan *request, 128),
		interval:  time.Minute,
	}
	ln, err := net.ListenUDP("udp", c.Local.IntranetAddr)
	if err != nil {
		log.Panic(err)
	}
	c.Local.IntranetAddr, _ = net.ResolveUDPAddr("udp", ln.LocalAddr().String())
	c.ln = ln
	go c.listen()
	go c.register()
	go c.handleReq()
	return c
}

func (c *Client) listen() {
	buf := make([]byte, 1024)
	for {
		n, addr, err := c.ln.ReadFromUDP(buf)
		if err != nil {
			log.Error(err)
			continue
		}
		cmd := buf[0]
		data := buf[1:n]
		req := &request{
			Type: cmd,
			Data: data,
			Src:  addr,
		}
		log.Debugf("read req: %v from %s", string(req.Data), req.Src)
		switch cmd {
		case CmdServerRegisterReply, CmdServerConnReply, CmdServerPushConn:
			c.ServerReq <- req
		case CmdPeerConn, CmdPeerData, CmdPeerConnAck:
			c.PeerReq <- req
		}
	}
}

func (c *Client) register() {
	c.sendRequest(&request{
		Type: CmdClientRegister,
		Data: []byte(c.Local.Id),
		Dst:  c.Server.PublicAddr,
	})
	go c.heartBeat(c.Server.PublicAddr)
}

func (c *Client) handleReq() {
	go c.handleServer()
	go c.handlePeer()
}

func (c *Client) handleServer() {
	for req := range c.ServerReq {
		switch req.Type {
		case CmdServerRegisterReply:
			ip := string(req.Data)
			addr, err := net.ResolveUDPAddr("udp", ip)
			if err != nil {
				log.Error(err)
				continue
			}
			c.Local.PublicAddr = addr
			log.Infof("listen at %s", c.Local)
		case CmdServerConnReply, CmdServerPushConn:
			ss := strings.Split(string(req.Data), ",")
			dstId, ip := ss[0], ss[1]
			dstAddr, err := net.ResolveUDPAddr("udp", ip)
			if err != nil {
				log.Error(err)
				continue
			}
			c.conn(dstId, dstAddr)
		default:
			log.Errorf("Error req type: %v", req.Type)
		}
	}
}

func (c *Client) handlePeer() {
	for req := range c.PeerReq {
		switch req.Type {
		case CmdPeerConn:
			c.connAck(req.Src)
			go c.heartBeat(req.Src)
			log.Infof("build conn to %s", req.Src)
		case CmdPeerConnAck:
			go c.heartBeat(req.Src)
			log.Infof("build conn to %s", req.Src)
		case CmdPeerData:
			fmt.Printf("[%s] %s\n", c.IpToId[req.Src.String()], string(req.Data))
		default:
			log.Errorf("Error req type: %v", req.Type)
		}
	}
}

func (c *Client) conn(id string, addr *net.UDPAddr) {
	if _, ok := c.PeerMap[id]; !ok {
		c.PeerMap[id] = &Peer{
			Id:         id,
			PublicAddr: addr,
		}
	}
	c.IpToId[addr.String()] = id
	c.sendRequest(&request{
		Type: CmdPeerConn,
		Dst:  addr,
	})

}

func (c *Client) connAck(addr *net.UDPAddr) {
	c.sendRequest(&request{
		Type: CmdPeerConnAck,
		Dst:  addr,
	})
}

func (c *Client) writeUdp(data []byte, addr *net.UDPAddr) error {
	n, err := c.ln.WriteToUDP(data, addr)
	if err != nil {
		return fmt.Errorf("write [%v] to [%v] err: %v", string(data), addr.String(), err)
	} else if n != len(data) {
		return fmt.Errorf("write [%v] to [%v] err: need write: %v , actual write: %v", string(data), addr.String(), len(data), n)
	}
	return nil
}

func (c *Client) SendTo(id, msg string) {
	peer, ok := c.PeerMap[id]
	if !ok {
		log.Errorf("%v not in peer map", id)
		return
	}
	c.sendRequest(&request{
		Type: CmdPeerData,
		Data: []byte(msg),
		Dst:  peer.PublicAddr,
	})
}

func (c *Client) Conn(id string) {
	c.sendRequest(&request{
		Type: CmdClientConn,
		Data: []byte(id),
		Dst:  c.Server.PublicAddr,
	})
}

func (c *Client) heartBeat(dst *net.UDPAddr) {
	for range time.Tick(c.interval) {
		c.sendRequest(&request{
			Type: CmdHeartBeat,
			Dst:  dst,
		})
		log.Debugf("heartbeat to %s", dst.String())

	}
}

func (c *Client) sendRequest(req *request) {
	data := append([]byte{req.Type}, req.Data...)
	err := c.writeUdp(data, req.Dst)
	if err != nil {
		log.Error(err)
		if req.Type == CmdClientRegister {
			panic("register error")
		}
	}
}
