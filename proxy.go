package main

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
)

type Proxy struct {
	stopCh   chan struct{} // Channel to stop proxy
	stopOnce sync.Once

	config       Config
	serverConfig *ssh.ServerConfig // SSH downstream configuration
	listener     net.Listener      // TCP listener

	summaries sync.WaitGroup // Force stop to wait on running log summaries
}

// Returns a proxy based on a given valid proxy config file path
func NewProxy(configPath string) (*Proxy, error) {
	config, err := ParseConfig(configPath)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	serverConfig, err := config.BuildServerConfig()
	if err != nil {
		return nil, err
	}

	proxy := &Proxy{
		stopCh:   make(chan struct{}),
		stopOnce: sync.Once{},

		config:       *config,
		serverConfig: serverConfig,
		listener:     nil,

		summaries: sync.WaitGroup{},
	}

	return proxy, nil
}

// Stops proxy
//
// Only stops once just in case. Waits on log summaries
func (proxy *Proxy) Stop() {
	// Stops al of proxy's main behavior
	proxy.stopOnce.Do(func() {
		close(proxy.stopCh)

		if proxy.listener != nil {
			proxy.listener.Close()
		}
	})
	// Waits on finishing summaries before returning (for Main)
	proxy.summaries.Wait()
	log.Println("Proxy stopped")
}

// Builds SSH proxy-as-server configuration for client users based on proxy's config file
func (config *Config) BuildServerConfig() (*ssh.ServerConfig, error) {
	serverConfig := &ssh.ServerConfig{}

	serverConfig.PublicKeyCallback = func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		username := conn.User()
		user, ok := config.Users[username]
		if !ok {
			return nil, fmt.Errorf("Unknown user")
		}

		if user.PublicKey == "" {
			return nil, fmt.Errorf("No public key configured for user %q", username)
		}

		authorizedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(user.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("Invalid public key for %q: %w", username, err)
		}
		if !bytes.Equal(authorizedKey.Marshal(), key.Marshal()) {
			return nil, fmt.Errorf("Invalid public key")
		}

		return nil, nil
	}

	serverConfig.PasswordCallback = func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		username := conn.User()
		user, ok := config.Users[username]
		if !ok {
			return nil, fmt.Errorf("Unknown user")
		}

		if user.Password == "" {
			return nil, fmt.Errorf("No password configured for user %q", username)
		}

		// Just in case time taken to compare reveals any password information
		if subtle.ConstantTimeCompare([]byte(user.Password), password) != 1 {
			return nil, fmt.Errorf("Invalid password")
		}

		return nil, nil
	}

	// Host key presented to client users
	hostKeyBytes, err := os.ReadFile("./keys/proxy_host_key")
	if err != nil {
		return nil, fmt.Errorf("read host key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(hostKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse host key: %w", err)
	}

	serverConfig.AddHostKey(signer)

	return serverConfig, nil
}

// Builds SSH proxy-as-client configuration for upstream server based on proxy's config file
func (config *Config) BuildClientConfig() (*ssh.ClientConfig, error) {
	clientConfig := &ssh.ClientConfig{}

	clientConfig.User = config.Upstream.Username
	var authMethods []ssh.AuthMethod

	if config.Upstream.PrivateKeyFile != "" {
		privateKeyBytes, err := os.ReadFile(config.Upstream.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("Read upstream private key file error: %w", err)
		}

		signer, err := ssh.ParsePrivateKey(privateKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("Parse upstream private key file error: %w", err)
		}

		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if config.Upstream.Password != "" {
		authMethods = append(authMethods, ssh.Password(config.Upstream.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("Somehow no authentication method configured")
	}
	clientConfig.Auth = authMethods

	clientConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	return clientConfig, nil
}

// Listens for client TCP connections on `proxyPort` and handles them concurrently
//
// Terminates when proxy is stopped or when listener errors
func (proxy *Proxy) Start(proxyPort string) error {
	listener, err := net.Listen("tcp", proxyPort)
	if err != nil {
		return err
	}
	log.Printf("Proxy is listening on port %v", proxyPort)

	proxy.listener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-proxy.stopCh:
				return nil
			default:
				return err
			}
		}

		go proxy.handleConnection(conn)
	}
}

// Handles a SSH connection from a client user, authenticating user and connecting with both downstream and upstream
//
// Terminates on error, or when the downstream connection closes. Session channels run concurrently and are not waited on.
func (proxy *Proxy) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Connect with downstream via SSH
	downstreamConn, chans, reqs, err := ssh.NewServerConn(conn, proxy.serverConfig)
	if err != nil {
		log.Printf("Downstream SSH handshake failed: %v", err)
		return
	}
	defer downstreamConn.Close()

	// Discard connection-level requests. Session requests are handled per channel.
	go ssh.DiscardRequests(reqs)

	log.Printf("Authenticated downstream user %s", downstreamConn.User())

	// Set up upstream configuration and address
	upstreamConfig, err := proxy.config.BuildClientConfig()
	if err != nil {
		log.Printf("Build upstream config error: %v", err)
		return
	}
	upstreamAddr := fmt.Sprintf("%s:%d", proxy.config.Upstream.Host, proxy.config.Upstream.Port)

	// Connect with upstream via SSH
	upstreamConn, err := ssh.Dial("tcp", upstreamAddr, upstreamConfig)
	if err != nil {
		log.Printf("Upstream SSH connection failed: %v", err)
		return
	}
	defer upstreamConn.Close()

	// Handle each SSH session channel from downstream client
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "Unknown channel type")
			continue
		}

		go proxy.handleSession(newChannel, upstreamConn, downstreamConn.User())
	}
}

// Handles a potential SSH session channel from downstream client by starting a matching channel with upstream server
//
// Terminates if error or when [Proxy.forwardSessionData] terminates
func (proxy *Proxy) handleSession(channel ssh.NewChannel, upstreamConn *ssh.Client, username string) {
	// Open matching session channel on upstream
	upstreamChannel, upstreamReqs, err := upstreamConn.OpenChannel("session", channel.ExtraData())
	if err != nil {
		log.Printf("Failed to match downstream session: %v", err)
		channel.Reject(ssh.ConnectionFailed, "Failed to match downstream session")
		return
	}
	defer upstreamChannel.Close()

	// Accept downstream's session channel
	downstreamChannel, downstreamReqs, err := channel.Accept()
	if err != nil {
		log.Printf("Failed to accept downstream session: %v", err)
		return
	}
	defer downstreamChannel.Close()

	// Create a session logger to log downstream stdin input
	sessionLogger, err := NewSessionLogger(proxy.config.LogDir, username, proxy.config.LLM)
	if err != nil {
		log.Printf("Failed to create session logger: %v", err)
		return
	}
	// A session implies there should be a summary of its log! Done asynchronously
	proxy.summaries.Add(1)
	defer func() {
		go func() {
			defer proxy.summaries.Done()
			if err := sessionLogger.Summarize(); err != nil {
				log.Printf("Summarize session log error: %v", err)
			}
		}()
	}()

	// Concurrently forward both SSH session requests and data between client and upstream server bi-directionally
	go proxy.forwardSessionRequests(downstreamReqs, upstreamReqs, downstreamChannel, upstreamChannel)
	proxy.forwardSessionData(downstreamChannel, upstreamChannel, sessionLogger)
}

// Forwards SSH session requests (any and all) between client and upstream server
//
// Terminates when Stop is called, or when both downstream and upstream request channels have closed
func (proxy *Proxy) forwardSessionRequests(downstreamReqs <-chan *ssh.Request, upstreamReqs <-chan *ssh.Request, downstreamChannel ssh.Channel, upstreamChannel ssh.Channel) {
	for {
		if downstreamReqs == nil && upstreamReqs == nil {
			return
		}
		select {
		// Client -> upstream server
		case req, ok := <-downstreamReqs:
			if !ok {
				downstreamReqs = nil
			} else {
				ok, err := upstreamChannel.SendRequest(req.Type, req.WantReply, req.Payload)
				if req.WantReply {
					req.Reply(err == nil && ok, nil)
				}
			}
		// Upstream server -> client
		case req, ok := <-upstreamReqs:
			if !ok {
				upstreamReqs = nil
			} else {
				ok, err := downstreamChannel.SendRequest(req.Type, req.WantReply, req.Payload)
				if req.WantReply {
					req.Reply(err == nil && ok, nil)
				}
			}
		case <-proxy.stopCh:
			return
		}
	}
}

// Forwards SSH session data (stdin, stdout, stderr) and logs client stdin via a session logger
//
// Terminates when stdout, stderr, and stdin finish.
func (proxy *Proxy) forwardSessionData(downstreamChannel ssh.Channel, upstreamChannel ssh.Channel, sessionLogger *SessionLogger) {
	var outputs sync.WaitGroup
	input := make(chan struct{})
	session := make(chan struct{})
	defer close(session)

	outputs.Add(2)

	// Forward stdin (downstream -> upstream + session log)
	go func() {
		defer close(input)
		sessionLogger.ForwardInput(downstreamChannel, upstreamChannel)
		upstreamChannel.CloseWrite()
	}()
	// Forward stdout (upstream -> downstream)
	go func() {
		defer outputs.Done()
		io.Copy(downstreamChannel, upstreamChannel)
	}()
	// Forward stderr (upstream -> downstream)
	go func() {
		defer outputs.Done()
		io.Copy(downstreamChannel.Stderr(), upstreamChannel.Stderr())
	}()

	// If proxy stops, close upstream and downstream channels, which stops all session data goroutines
	go func() {
		select {
		case <-proxy.stopCh:
			upstreamChannel.Close()
			downstreamChannel.Close()
		case <-session:
		}
	}()
	outputs.Wait()
	downstreamChannel.Close()
	<-input
}
