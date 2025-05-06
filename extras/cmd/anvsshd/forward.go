package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"sync"

	"golang.org/x/crypto/ssh"
)

func listenAndForward(req *ssh.Request, addr string, port uint32, sshConn ssh.Conn) error {
	sendReply := func(b bool) {
		if req.WantReply {
			req.Reply(b, nil)
		}
	}

	ipaddr, err := netip.ParseAddr(addr)
	if err != nil {
		sendReply(false)
		return fmt.Errorf("Invalid IP address: %v", err)
	}

	if !ipaddr.Is4() {
		sendReply(false)
		return fmt.Errorf("Non-IPv4 addresses are not supported")
	}

	combined := fmt.Sprintf("%s:%d", addr, port)

	listener, err := net.Listen("tcp", combined)
	if err != nil {
		sendReply(false)
		return fmt.Errorf("Listening on address %s failed: %v", combined, err)
	}
	
	replySent := false
	if port == 0 && req.WantReply {
		tcpAddr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			return fmt.Errorf("Cannot determine the local port we ended up listening on")
		}
		
		b := marshalUint32(tcpAddr.Port)
		replySent = true
		req.Reply(true, b)
	}

	log.Printf("Listening on %s to forward connections to the client", combined)
	go acceptAndForward(listener, sshConn)

	if !replySent {
		sendReply(true)
	}
	return nil
}

func acceptAndForward(listener net.Listener, sshConn ssh.Conn) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Accepting connection for fowarding failed: %v", err)
			continue
		}

		go forward(conn, sshConn)
	}
}

func forward(conn net.Conn, sshConn ssh.Conn) {
	var openData struct {
		NewAddr    string
		NewPort    uint32
		ListenAddr string
		ListenPort uint32
	}

	tcpAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("Accepting connection for fowarding failed: cannot convert local address to TCP address. It is of type %T", conn.LocalAddr())
		return
	}

	openData.NewAddr = tcpAddr.IP.String()
	openData.NewPort = uint32(tcpAddr.Port)

	tcpAddr, ok = conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		log.Printf("Accepting connection for fowarding failed: cannot convert remote address to TCP address. It is of type %T", conn.RemoteAddr())
		return
	}

	openData.ListenAddr = tcpAddr.IP.String()
	openData.ListenPort = uint32(tcpAddr.Port)

	log.Printf("Accepted connection for fowarding from %s:%d", openData.NewAddr, openData.NewPort)

	msg := ssh.Marshal(openData)
	log.Printf("Sending forwarded-tcpip message:\n%s\n", hex.Dump(msg))

	channel, reqs, err := sshConn.OpenChannel("forwarded-tcpip", msg)
	if err != nil {
		log.Printf("Opening SSH channel to remote client failed: %v", err)
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	// The Request channel must be serviced.
	wg.Add(1)
	go func() {
		ssh.DiscardRequests(reqs)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		io.Copy(conn, channel)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		io.Copy(channel, conn)
		wg.Done()
	}()

}
