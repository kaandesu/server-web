package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type (
	Server struct {
		serverListener net.Listener
		userListener   net.Listener
		msgserverch    chan Message
		msgch          chan Message
		done           chan os.Signal
		servers        map[string]*Server
		clients        map[string]*Client
		addr           string
		apex           bool
		registered     bool
		capacity       int
	}
	Client struct {
		parentServer *Server
		con          net.Conn
		addr         string
	}
	Message struct {
		fromServer *Server
		fromClient *Client
		payload    []byte
	}
)

const (
	AuxAddr         = "127.0.0.1:"
	ApexAddr        = "127.0.0.1:3000"
	DefaultCapacity = 5
)

func NewServer(addr string, isApex bool) *Server {
	s := &Server{
		msgserverch: make(chan Message),
		msgch:       make(chan Message),
		done:        make(chan os.Signal),
		servers:     make(map[string]*Server),
		clients:     make(map[string]*Client),
		registered:  isApex,
		apex:        isApex,
		addr:        addr,
		capacity:    DefaultCapacity,
	}
	if !isApex {
		s.servers[ApexAddr] = nil
	}

	return s
}

func (s *Server) Start() {
	var err error
	s.serverListener, err = net.Listen("tcp", s.addr)
	if err != nil {
		slog.Error("Could not start the server", "err", err)
	}

	go s.DialServer()
	go s.acceptServer()
	go s.handleUserMessages()
	go s.handleServerMessage()

	signal.Notify(s.done, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
	<-s.done

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if err = s.Shutdown(ctx); err != nil {
		slog.Error("Could not stop the server", "err", err)
	}
}

func (s *Server) DialServer() {
	if s.apex {
		return
	}

	var (
		con net.Conn
		err error
	)

	for server := range s.servers {
		con, err = net.Dial("tcp", server)
		if err == nil {
			break
		}
	}

	fmt.Fprintf(con, "\n%+v\n", s)
	con.Close()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) acceptServer() {
	for {
		con, err := s.serverListener.Accept()
		if err != nil {
			break
		}
		s.handleServerConnection(con)
	}
}

func (s *Server) handleServerConnection(con net.Conn) {
	node := NewServer(con.RemoteAddr().String(), false)
	s.servers[node.addr] = node
	buffer := make([]byte, 2048) // magic number?
	for {
		n, err := con.Read(buffer)
		if err != nil {
			slog.Info("Server disconnected from server", "err", err)
			break
		}

		s.msgserverch <- Message{
			fromServer: node,
			fromClient: nil,
			payload:    buffer[:n],
		}

		// TODO: first message will contain info about:
		// - server list
		// - capacity
		// on this message, update the entry on this ones server list
		// Maybe protocol can have an "update" field, and we can handle
		// this stuff above while handling the messages???!!!!!!
		// flip the "registered" field once updated

	}
}

func (s *Server) handleUserMessages() {}

func (s *Server) handleServerMessage() {
	for msg := range s.msgserverch {
		slog.Info("[server][msg]", "msg", string(msg.payload), "from", msg.fromServer.addr)
	}
}

func main() {
	apexFlag := flag.Bool("apex", false, "--apex | --apex=true | --apex=false [default false]")
	flag.Parse()

	var server *Server
	if *apexFlag {
		server = NewServer(ApexAddr, true)
	} else {
		server = NewServer(AuxAddr, false)
	}
	slog.Info("Starting the server on", "addr", server.addr, "IsApex", server.apex)
	server.Start()
}
