package main

// https://www.rfc-editor.org/rfc/rfc4254#section-6.1

// Show processes with process group id: ps xao pid,ppid,pgid,sid,args

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/spf13/pflag"
	"golang.org/x/crypto/ssh"
)

var optListenAddr *string
var optAuthKeysFile *string
var optHostKeyFile *string

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
	pflag.PrintDefaults()
}

func main() {
	parseFlags()

	config := buildServerConfig()

	// Once a ServerConfig has been configured, connections can be
	// accepted.
	listener, err := net.Listen("tcp", *optListenAddr)
	if err != nil {
		log.Fatal("failed to listen for connection: ", err)
	}
	log.Printf("Listening on %s\n", *optListenAddr)

	acceptAndHandleConnections(listener, config)
}

func parseFlags() {
	defAuthKeysFile := "authorized_keys"
	defHostKeyFile := "host_key"

	home := os.Getenv("HOME")
	if home != "" {
		defAuthKeysFile = filepath.Join(home, ".ssh", defAuthKeysFile)
		defHostKeyFile = filepath.Join(home, ".ssh", defHostKeyFile)
	}

	optListenAddr = pflag.StringP("addr", "a", "0.0.0.0:5001", "What address and port to listen on")
	optAuthKeysFile = pflag.StringP("authkeys", "z", defAuthKeysFile, "file to load authorized keys from")
	optHostKeyFile = pflag.StringP("hostkey", "k", defHostKeyFile, "file containing host private key")

	pflag.Parse()
	pflag.Usage = usage
}

func acceptAndHandleConnections(listener net.Listener, config *ssh.ServerConfig) {
	for {
		nConn, err := listener.Accept()
		if err != nil {
			log.Println("failed to accept incoming connection: ", err)
			continue
		}

		go handleConnection(nConn, config)
	}
}

func handleConnection(nConn net.Conn, config *ssh.ServerConfig) {
	// Before use, a handshake must be performed on the incoming
	// net.Conn.
	conn, chans, reqs, err := ssh.NewServerConn(nConn, config)
	if err != nil {
		log.Println("failed to handshake: ", err)
		return
	}
	log.Printf("User logged in with key %s", conn.Permissions.Extensions["pubkey-fp"])

	var wg sync.WaitGroup
	defer wg.Wait()

	// The incoming Request channel must be serviced.
	wg.Add(1)
	go func() {
		handleRequests(conn, reqs)
		wg.Done()
	}()

	processChannels(chans)

}

func handleRequests(conn ssh.Conn, reqs <-chan *ssh.Request) {
	for req := range reqs {
		logSshRequest("connection", req)

		switch req.Type {
		case "tcpip-forward":
			processTcpForwardReq(conn, req)
		default:

			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// TODO: we need to close this listener at some point. I think we currently leak it.
func processTcpForwardReq(conn ssh.Conn, req *ssh.Request) {
	sendReply := func(b bool) {
		if req.WantReply {
			req.Reply(b, nil)
		}
	}

	var tcpFordwardReq struct {
		Addr string
		Port uint32
	}

	err := ssh.Unmarshal(req.Payload, &tcpFordwardReq)
	if err != nil {
		log.Printf("Unmarshalling TCP Forward request failed: %v\n", err)
		sendReply(false)
		return
	}

	// listenAndForward handles sending the reply
	err = listenAndForward(req, tcpFordwardReq.Addr, tcpFordwardReq.Port, conn)
	if err != nil {
		log.Printf("Handling tcpip-forward request failed: %v\n", err)
		return
	}
}

func buildServerConfig() *ssh.ServerConfig {
	// Public key authentication is done by comparing
	// the public key of a received connection
	// with the entries in the authorized_keys file.
	authorizedKeysBytes, err := os.ReadFile(*optAuthKeysFile)
	if err != nil {
		log.Fatalf("Failed to load authorized_keys, err: %v", err)
	}

	authorizedKeysMap := map[string]bool{}
	for len(authorizedKeysBytes) > 0 {
		pubKey, _, _, rest, err := ssh.ParseAuthorizedKey(authorizedKeysBytes)
		if err != nil {
			log.Fatal(err)
		}

		authorizedKeysMap[string(pubKey.Marshal())] = true
		authorizedKeysBytes = rest
	}

	// An SSH server is represented by a ServerConfig, which holds
	// certificate details and handles authentication of ServerConns.
	config := &ssh.ServerConfig{
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if authorizedKeysMap[string(pubKey.Marshal())] {
				return &ssh.Permissions{
					// Record the public key used for authentication.
					Extensions: map[string]string{
						"pubkey-fp": ssh.FingerprintSHA256(pubKey),
					},
				}, nil
			}
			return nil, fmt.Errorf("unknown public key for %q", c.User())
		},
	}

	privateBytes, err := os.ReadFile(*optHostKeyFile)
	if err != nil {
		log.Fatal("Failed to load private key: ", err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key: ", err)
	}
	config.AddHostKey(private)

	return config
}

func processChannels(chans <-chan ssh.NewChannel) {
	// Service the incoming Channel channel. Each of these correspond to
	// "session" from the RFC, I believe.
	for newChannel := range chans {
		go processNewChannel(newChannel)
	}

}

func processNewChannel(newChannel ssh.NewChannel) {
	log.Printf("New channel of type: %s\n", newChannel.ChannelType())
	if newChannel.ChannelType() != "session" {
		newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
		return
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("Could not accept channel: %v", err)
		return
	}

	defer func() {
		err = channel.Close()
		if err != nil {
			log.Printf("error during channel close: %v\n", err)
		}
		log.Printf("closed channel\n")
	}()

	initialChannelProps, ok := processInitialRequestsForExec(channel, requests)
	if !ok {
		return
	}

	cmd := exec.Command("bash", "-c", initialChannelProps.cmd)
	log.Printf("Running command: bash -c '%s'\n", initialChannelProps.cmd)

	cmd.Stdin = channel
	cmd.Stdout = channel
	cmd.Stderr = channel
	// Set the tree of processes we create to all have the same PGID, so that
	// we can kill the PGID to kill all processes
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	killedProcess := make(chan struct{}, 1)
	go processOngoingRequestsForExec(channel, cmd, requests, killedProcess)

	err = cmd.Start()
	if err != nil {
		log.Printf("Starting command failed with error: %v\n", err)
		sendExitStatus(channel, 1)
		return
	}

	var exitDueToSignal bool
	err = cmd.Wait()
	if err != nil {
		if errorIndicatesProcessExitedDueToSignal(err) {
			log.Printf("Command exited due to signal\n")
			exitDueToSignal = true
		} else {
			log.Printf("Command failed with error: %v %T %#v\n", err, err, err)
		}
	} else {
		log.Printf("Command completed successfully\n")
	}

	if exitDueToSignal && channelWasWrittenTo(killedProcess) {
		log.Printf("More specifically, command exited due to signal we sent\n")
		// If we killed the process, send the signal code + 128 as per advanced bash scripting guide
		// https://stackoverflow.com/questions/39269536/what-should-my-programs-exit-code-be-if-i-caught-a-signal
		sendExitStatus(channel, 128+9)
	} else {
		sendExitStatusFromError(channel, err)
	}
	log.Printf("Sent exit status\n")
}

func processOngoingRequestsForExec(channel ssh.Channel, cmd *exec.Cmd, requests <-chan *ssh.Request, killedProcess chan struct{}) {
	for req := range requests {
		logSshRequest("session", req)

		switch req.Type {
		case "signal":
			processSignalReq(channel, cmd, req, killedProcess)
		}
	}
}

func processSignalReq(channel ssh.Channel, cmd *exec.Cmd, req *ssh.Request, killedProcess chan struct{}) {
	sendReply := func(b bool) {
		if req.WantReply {
			req.Reply(b, nil)
		}
	}

	// Send a response as if our signal call was successful. We do this now because
	// as soon as the process receives a signal, exec.Cmd closes the stdout and stderr io.Writers
	// before exec.Cmd.Wait call exits, so there is no way to send on the channel after that.
	// Supported enum values are in https://datatracker.ietf.org/doc/html/rfc4254#section-6.10
	err := sendExitSignal(channel, "KILL")
	if err != nil {
		log.Printf("Sending exit-signal message failed: %v\n", err)
	} else {
		killedProcess <- struct{}{}
	}

	log.Printf("Sent exit-signal to client\n")
	// Send a signal to the process' process group (see kill(2))
	if cmd.Process == nil {
		log.Printf("Not sending exit-signal since process seems to have already exited\n")
		return
	}

	log.Printf("Sending kill to process group %d\n", cmd.Process.Pid)
	err = syscall.Kill(-cmd.Process.Pid, syscall.Signal(9))
	if err != nil {
		log.Printf("Killing process failed: %v\n", err)
		sendReply(false)
		return
	}
	sendReply(true)
}

type initialChannelProps struct {
	env map[string]string
	cmd string
}

func processInitialRequestsForExec(channel ssh.Channel, requests <-chan *ssh.Request) (props initialChannelProps, ok bool) {
	ok = true

loop:
	// Process requests until we see one of "shell", "exec" or "subsystem" as per RFC 4254 section 6.5
	for req := range requests {
		logSshRequest("session", req)

		switch req.Type {
		case "env":
			handleEnvRequest(req)
		case "shell", "subsystem":
			if req.WantReply {
				req.Reply(false, nil)
			}
			channel.Close()
			ok = false
			return
		case "exec":
			var err error
			props.cmd, err = unmarshalString(req.Payload)
			if err != nil {
				log.Printf("Unmarshaling exec string failed: %v", err)
				if req.WantReply {
					req.Reply(false, nil)
				}
				ok = false
				return
			}
			if req.WantReply {
				req.Reply(true, nil)
			}
			break loop
		default:
			log.Printf("Received unexpected request on channel initialization: %s", req.Type)
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}

	if props.cmd == "" {
		log.Printf("No exec message was seen on channel initialization")
		ok = false
	}
	return
}

func logSshRequest(prefix string, r *ssh.Request) {
	log.Printf("%s Request. type: %s want reply: %v payload:\n%s\n", prefix, r.Type, r.WantReply, hex.Dump(r.Payload))
}

func handleEnvRequest(req *ssh.Request) {
	sendReply := func(b bool) {
		if req.WantReply {
			req.Reply(b, nil)
		}
	}

	var env struct {
		Name, Value string
	}

	err := ssh.Unmarshal(req.Payload, &env)
	if err != nil {
		log.Printf("Unmarshalling env var failed: %v\n", err)
		sendReply(false)
		return
	}

	err = os.Setenv(env.Name, env.Value)
	if err != nil {
		log.Printf("Setting env var failed: %v\n", err)
		sendReply(false)
		return
	}
	sendReply(true)
}

func unmarshalString(data []byte) (string, error) {
	var str struct {
		Val string
	}

	err := ssh.Unmarshal(data, &str)
	if err != nil {
		return "", err
	}

	return str.Val, nil
}

func marshalUint32(v int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

func marshalExecError(err error) []byte {
	// RFC 4254 says exit-status values are a uint32 (section 6.10) and I think this is encoded as
	// 1 4-byte network-endian value (RFC 4251 section 5)

	if err == nil {
		return marshalUint32(0)
	}

	switch e := err.(type) {
	case *exec.ExitError:
		log.Printf("Marshaling exit code %d\n", e.ExitCode())
		return marshalUint32(e.ExitCode())
	default:
		return marshalUint32(1)
	}
}

func sendExitStatusFromError(channel ssh.Channel, execErr error) {
	b := marshalExecError(execErr)
	channel.SendRequest("exit-status", false, b)
}

func sendExitStatus(channel ssh.Channel, i int) {
	b := marshalUint32(i)
	channel.SendRequest("exit-status", false, b)
}

func marshalExitSignal(name string) []byte {

	m := struct {
		SignalName string
		CoreDumped bool
		ErrorMsg   string
		Language   string
	}{
		name,
		false,
		"Killed by signal sent from ssh client",
		"en",
	}

	return ssh.Marshal(&m)
}

func sendExitSignal(channel ssh.Channel, name string) error {
	b := marshalExitSignal(name)
	var err error
	_, err = channel.SendRequest("exit-signal", false, b)
	return err
}

func errorIndicatesProcessExitedDueToSignal(err error) bool {
	if err == nil {
		return false
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}

	state := exitErr.ProcessState
	return state.ExitCode() == -1
}

func channelWasWrittenTo(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
	}
	return false
}
