package p2p

import (
	"fmt"
	"net"

	log "github.com/sirupsen/logrus"
)

var buf = make([]byte, 1024)

const (
	CmdClientRegister      = 0x01
	CmdServerRegisterReply = 0x02
	CmdClientConn          = 0x03
	CmdServerConnReply     = 0x04
	CmdServerPushConn      = 0x05
	CmdPeerConn            = 0x06
	CmdPeerData            = 0x07
	CmdPeerConnAck         = 0x08
	CmdHeartBeat           = 0x09
)

type clientReq struct {
	Type byte
	Id   string
	Src  *Peer
}

type Server struct {
	PeerMap   map[string]*Peer
	IpToId    map[string]string
	ClientReq chan *clientReq
	ln        *net.UDPConn
}

func NewServer() *Server {
	s := &Server{
		PeerMap:   make(map[string]*Peer),
		IpToId:    make(map[string]string),
		ClientReq: make(chan *clientReq, 128),
	}
	return s
}

func (s *Server) Listen() {
	l, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 9104,
	})
	if err != nil {
		log.Error(err)
	}
	s.ln = l
	go s.HandleReq()
	for {
		n, addr, err := l.ReadFromUDP(buf)
		if err != nil {
			log.Error(err)
			continue
		}
		log.Debugf("[from %v]: %v", addr.String(), string(buf[:n]))
		opt := buf[0]
		id := string(buf[1:n])
		if opt == CmdClientRegister {
			s.register(id, addr)
		} else if opt == CmdClientConn {
			s.conn(id, addr)
		}
	}
}

func (s *Server) register(id string, addr *net.UDPAddr) {
	req := &clientReq{
		Type: CmdClientRegister,
		Id:   id,
		Src: &Peer{
			Id:         id,
			PublicAddr: addr,
		},
	}
	s.ClientReq <- req
}

func (s *Server) conn(id string, addr *net.UDPAddr) {
	srcId, ok := s.IpToId[addr.String()]
	if !ok {
		log.Errorf("%v try conn %v, but not register", addr.String(), id)
		return
	}
	peer, ok := s.PeerMap[srcId]
	if !ok {
		log.Errorf("%v:%v try conn  %v, but not register", srcId, addr.String(), id)
		return
	}
	req := &clientReq{
		Type: CmdClientConn,
		Id:   id,
		Src:  peer,
	}
	s.ClientReq <- req
}

func (s *Server) HandleReq() {
	for req := range s.ClientReq {
		switch req.Type {
		case CmdClientRegister:
			s.handleRegister(req)
		case CmdClientConn:
			s.handleConn(req)
		}
	}
}

func (s *Server) handleRegister(req *clientReq) {
	peer := &Peer{
		Id:         req.Id,
		PublicAddr: req.Src.PublicAddr,
	}
	s.PeerMap[req.Id] = peer
	s.IpToId[req.Src.PublicAddr.String()] = req.Id
	reply := []byte{CmdServerRegisterReply}
	reply = append(reply, []byte(req.Src.PublicAddr.String())...)
	n, err := s.ln.WriteToUDP(reply, req.Src.PublicAddr)
	if err != nil {
		log.Errorf("write [%v] to [%v][%v] err: %v", string(reply), req.Id, req.Src.PublicAddr.String(), err)
		return
	} else if n != len(reply) {
		log.Errorf("write [%v] to [%v][%v] err: need write: %v , actual write: %v", string(reply), req.Id, req.Src.PublicAddr.String(), len(reply), n)
		return
	}
	log.Infof("%v register at %v", req.Id, req.Src.PublicAddr.String())
}

func (s *Server) handleConn(req *clientReq) {
	srcPeer := req.Src
	dstPeer, ok := s.PeerMap[req.Id]
	if !ok {
		log.Errorf("[%v] try conn [%v], but dst not register", req.Src.PublicAddr.String(), req.Id)
		return
	}
	replyToSrc := []byte{CmdServerConnReply}
	replyToSrc = append(replyToSrc, []byte(fmt.Sprintf("%s,%s", dstPeer.Id, dstPeer.PublicAddr.String()))...)
	err := s.writeUdp(replyToSrc, srcPeer.PublicAddr)
	if err != nil {
		log.Error(err)
		return
	}
	replyToDst := []byte{CmdServerPushConn}
	replyToDst = append(replyToDst, []byte(fmt.Sprintf("%s,%s", srcPeer.Id, srcPeer.PublicAddr.String()))...)
	err = s.writeUdp(replyToDst, dstPeer.PublicAddr)
	if err != nil {
		log.Error(err)
		return
	}
	log.Infof("%s try conn to %s", srcPeer, dstPeer)
}

func (s *Server) writeUdp(data []byte, addr *net.UDPAddr) error {
	n, err := s.ln.WriteToUDP(data, addr)
	if err != nil {
		return fmt.Errorf("write [%v] to [%v] err: %v", string(data), addr.String(), err)
	} else if n != len(data) {
		return fmt.Errorf("write [%v] to [%v] err: need write: %v , actual write: %v", string(data), addr.String(), len(data), n)
	}
	return nil
}
