package serv

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lulugyf/sshserv/dataprovider"
	"github.com/lulugyf/sshserv/hh"
	"github.com/lulugyf/sshserv/logger"
	"github.com/lulugyf/sshserv/sftp"
	"github.com/lulugyf/sshserv/utils"
	"golang.org/x/crypto/ssh"
)


const defaultPrivateKeyName = "id_rsa"


// Configuration for the SFTP server
type Configuration struct {
	// Identification string used by the server
	Banner string `json:"banner" mapstructure:"banner"`
	// The port used for serving SFTP requests
	BindPort int `json:"bind_port" mapstructure:"bind_port"`
	// The address to listen on. A blank value means listen on all available network interfaces.
	BindAddress string `json:"bind_address" mapstructure:"bind_address"`
	// Maximum idle timeout as minutes. If a client is idle for a time that exceeds this setting it will be disconnected
	IdleTimeout int `json:"idle_timeout" mapstructure:"idle_timeout"`
	// Maximum number of authentication attempts permitted per connection.
	// If set to a negative number, the number of attempts are unlimited.
	// If set to zero, the number of attempts are limited to 6.
	MaxAuthTries int `json:"max_auth_tries" mapstructure:"max_auth_tries"`
	// Umask for new files
	Umask string `json:"umask" mapstructure:"umask"`
	// UploadMode 0 means standard, the files are uploaded directly to the requested path.
	// 1 means atomic: the files are uploaded to a temporary path and renamed to the requested path
	// when the client ends the upload. Atomic mode avoid problems such as a web server that
	// serves partial files when the files are being uploaded.
	UploadMode int `json:"upload_mode" mapstructure:"upload_mode"`
	// Actions to execute on SFTP create, download, delete and rename
	Actions Actions `json:"actions" mapstructure:"actions"`
	// Keys are a list of host keys
	Keys []Key `json:"keys" mapstructure:"keys"`
	// IsSCPEnabled determines if experimental SCP support is enabled.
	// We have our own SCP implementation since we can't rely on scp system
	// command to properly handle permissions, quota and user's home dir restrictions.
	// The SCP protocol is quite simple but there is no official docs about it,
	// so we need more testing and feedbacks before enabling it by default.
	// We may not handle some borderline cases or have sneaky bugs.
	// Please do accurate tests yourself before enabling SCP and let us known
	// if something does not work as expected for your use cases
	IsSCPEnabled bool `json:"enable_scp" mapstructure:"enable_scp"`

	// If Default open full functions ? (shell / LocalPortForward / RemotePortForward)
	FullFunc bool `json:"full_func" mapstructure:"full_func"`

	// use HDFS as backend store, format: {user}@{namenodes or conf_dir}
	HDFS string `json:"hdfs" mapstructure:"hdfs"`
	// hosts list:  host-172-18-231-22,172.18.231.22 host-172-18-231-25,172.18.231.25 host-172-18-231-27,172.18.231.27 host-172-18-231-19,172.18.231.19 host-172-18-231-20,172.18.231.20
	HDFSHosts string `json:"hdfs_hosts" mapstructure:"hdfs_hosts"`

	BasePubkey string `json:"_base_pubkey" mapstructure:"_base_pubkey"`

	_basePubKey string
	_baseUser string
}

// Key contains information about host keys
type Key struct {
	// The private key path relative to the configuration directory or absolute
	PrivateKey string `json:"private_key" mapstructure:"private_key"`
}

// Initialize the SFTP server and add a persistent listener to handle inbound SFTP connections.
func (c *Configuration) Initialize(configDir string) error {
	umask, err := strconv.ParseUint(c.Umask, 8, 8)
	if err == nil {
		utils.SetUmask(int(umask), c.Umask)
	} else {
		logger.Warn(logSender, "error reading umask, please fix your config file: %v", err)
		logger.WarnToConsole("error reading umask, please fix your config file: %v", err)
	}
	serverConfig := &ssh.ServerConfig{
		NoClientAuth: false,
		MaxAuthTries: c.MaxAuthTries,
		PasswordCallback: func(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			sp, err := c.validatePasswordCredentials(conn, pass)
			if err != nil {
				return nil, errors.New("could not validate credentials")
			}

			return sp, nil
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			sp, err := c.validatePublicKeyCredentials(conn, string(pubKey.Marshal()))
			if err != nil {
				return nil, errors.New("could not validate credentials")
			}

			return sp, nil
		},
		ServerVersion: "SSH-2.0-" + c.Banner,
	}

	err = c.checkHostKeys(configDir)
	if err != nil {
		return err
	}

	for _, k := range c.Keys {
		privateFile := k.PrivateKey
		if !filepath.IsAbs(privateFile) {
			privateFile = filepath.Join(configDir, privateFile)
		}
		logger.Info(logSender, "Loading private key: %s", privateFile)

		privateBytes, err := ioutil.ReadFile(privateFile)
		if err != nil {
			return err
		}

		private, err := ssh.ParsePrivateKey(privateBytes)
		if err != nil {
			return err
		}

		// Add private key to the server configuration.
		serverConfig.AddHostKey(private)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", c.BindAddress, c.BindPort))
	if err != nil {
		logger.Warn(logSender, "error starting listener on address %s:%d: %v", c.BindAddress, c.BindPort, err)
		return err
	}

	actions = c.Actions
	uploadMode = c.UploadMode
	logger.Info(logSender, "server listener registered address: %v", listener.Addr().String())
	if c.IdleTimeout > 0 {
		startIdleTimer(time.Duration(c.IdleTimeout) * time.Minute)
	}

	c._initBaseKey()
	for {
		conn, _ := listener.Accept()
		if conn != nil {
			go c.AcceptInboundConnection(conn, serverConfig)
		}
	}
}

func (c *Configuration) _initBaseKey() {
	if strings.Index(c.BasePubkey, "ssh-") >= 0 {
		c._baseUser = "_base_"
		s, _, _, _, err := ssh.ParseAuthorizedKey([]byte(c.BasePubkey))
		if err == nil {
			c._basePubKey = string(s.Marshal())
			//fmt.Printf("parse succ [%v]\n", c._basePubKey)
		}else{
			fmt.Printf("---basePubkey parse failed [%v]\n", err)
		}
	}
}
func (c *Configuration) _checkBaseKey(user, pubkey string) *ssh.Permissions {
	//fmt.Printf("_checkBaseKey: user[%s] [%v] [%v]\n",
	//	user,
	//	base64.StdEncoding.EncodeToString([]byte(c._basePubKey) ),
	//	base64.StdEncoding.EncodeToString([]byte(pubkey) ))
	if user == c._baseUser && pubkey == c._basePubKey {
		p := &ssh.Permissions{}
		p.Extensions = make(map[string]string)
		p.Extensions["user"] = `{"id":1,"username":"_base_","permissions":["*"],"password":"","home_dir":"/","uid":0,"gid":0,
"max_sessions":0,"quota_size":0,"quota_files":0,"used_quota_size":0,"used_quota_files":0,"last_quota_update":0,"upload_bandwidth":0,"download_bandwidth":0}`
		return p
	}
	return nil
}

// AcceptInboundConnection handles an inbound connection to the server instance and determines if the request should be served or not.
func (c *Configuration) AcceptInboundConnection(conn net.Conn, config *ssh.ServerConfig) {
	//fmt.Printf("---------AcceptInboundConnection \n")
	defer conn.Close()

	// Before beginning a handshake must be performed on the incoming net.Conn
	sconn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		logger.Warn(logSender, "failed to accept an incoming connection: %v", err)
		return
	}
	defer sconn.Close()

	logger.Debug(logSender, "accepted inbound connection, ip: %v", conn.RemoteAddr().String())

	var user dataprovider.User

	err = json.Unmarshal([]byte(sconn.Permissions.Extensions["user"]), &user)
	if err != nil {
		logger.Warn(logSender, "Unable to deserialize user info, cannot serv connection: %v", err)
		return
	}

	connectionID := hex.EncodeToString(sconn.SessionID())

	connection := Connection{
		ID:            connectionID,
		User:          user,
		ClientVersion: string(sconn.ClientVersion()),
		RemoteAddr:    conn.RemoteAddr(),
		StartTime:     time.Now(),
		lastActivity:  time.Now(),
		lock:          new(sync.Mutex),
		sshConn:       sconn,
	}

	//go ssh.DiscardRequests(reqs)

	logger.Debug(logSender, "   --client version: %s\n", sconn.ClientVersion())

	var forwardHandler *ForwardedTCPHandler = nil

	loop := true
	for loop {
		select {
		case r := <-reqs:
			if r == nil {
				loop = false
				break
			}
			logger.Debug(logRforward, "   reqs .. req.Type=%s\n", r.Type)
			switch (r.Type) {
			case "tcpip-forward":
				var payload []byte = nil
				ok := false
				if c.FullFunc && connection.User.HasPerm(dataprovider.PermTCPForward) {
					if forwardHandler == nil {
						forwardHandler = &ForwardedTCPHandler{forwards: make(map[string]net.Listener)}
					}
					ok, payload, _ = forwardHandler.handlePortforward(sconn, r)
				}
				if ok {
					r.Reply(true, payload)
				}else{
					r.Reply(false, nil)
				}
			}
		case newChannel := <-chans:
			if newChannel == nil {
				loop = false
				break
			}
			loop = c.iterChans(newChannel, sconn, connection)
		}
	}

	//Done close all port forwarding
	if forwardHandler != nil {
		for ln_addr, ln := range forwardHandler.forwards {
			ln.Close()
			logger.Debug(logRforward,"   R ln_addr [%s] closed\n", ln_addr)
		}
	}
	logger.Debug(logSender, "   ---------AcceptInboundConnection done \n")
}

func (c *Configuration) iterChans(newChannel ssh.NewChannel, sconn *ssh.ServerConn, connection Connection) bool {
	// If its not a session channel we just move on because its not something we
	// know how to handle at this point.
	logger.Debug(logSender,"  --- newChannel.ChannelType(): [%s] \n", newChannel.ChannelType())
	if newChannel.ChannelType() == "direct-tcpip" {
		if c.FullFunc && connection.User.HasPerm(dataprovider.PermTCPForward) {
			go HandleDirectTCPIP(sconn, newChannel)
			return true
		}else{
			logger.Warn(logLforward, "Denied -L port-forwarding of user %s", connection.User.Username)
			return false;
		}
	}

	if newChannel.ChannelType() != "session" {
		logger.Debug(logSender, "received an unknown channel type: %v", newChannel.ChannelType())
		newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
		return false
	}

	channel, requests, err := newChannel.Accept()
	if err != nil {
		logger.Warn(logSender, "could not accept a channel: %v", err)
		return false
	}

	// Channels have a type that is dependent on the protocol. For SFTP this is "subsystem"
	// with a payload that (should) be "sftp". Discard anything else we receive ("pty", "shell", etc)
	go handleSSHRequest(requests, channel, connection, c)

	return true
}


func (c *Configuration) handleSftpConnection(channel io.ReadWriteCloser, connection Connection) {
	addConnection(connection.ID, connection)
	// Create a new handler for the currently logged in user's server.
	var handler *sftp.Handlers = nil
	//fmt.Printf("------hdfs[%s] full_func[%v]\n", c.HDFS, c.FullFunc)
	var hdfsHandler *hh.HConnection = nil
	if c.HDFS == "" {
		handler = &sftp.Handlers{
			FileGet:  connection,
			FilePut:  connection,
			FileCmd:  connection,
			FileList: connection,
		}
	}else{
		hdfsHandler = hh.NewHandler(connection, connection.User, connection.ID, connection.protocol)
		cc := strings.Split(c.HDFS, "@")
		err := hdfsHandler.MkHdfsClient(cc[1], cc[0], c.HDFSHosts)
		if err != nil {
			fmt.Printf("---connect hdfs failed: %v\n", err)
		}else {
			handler = &sftp.Handlers{
				FileGet:  hdfsHandler,
				FilePut:  hdfsHandler,
				FileCmd:  hdfsHandler,
				FileList: hdfsHandler,
			}
		}
	}

	if handler != nil {
		// Create the server instance for the channel using the handler we created above.
		server := sftp.NewRequestServer(channel, *handler)

		if err := server.Serve(); err == io.EOF {
			logger.Debug(logSender, "connection closed, id: %v", connection.ID)
			server.Close()
		} else if err != nil {
			logger.Error(logSender, "sftp connection closed with error id %v: %v", connection.ID, err)
		}
	}else{
		fmt.Printf("can not handle sftp!!!\n")
		channel.Close()
	}

	removeConnection(connection.ID)
	//fmt.Printf("handle sftp finished.\n")
	if hdfsHandler != nil {
		hdfsHandler.Close()
	}
}

func loginUser(user dataprovider.User, c *Configuration) (*ssh.Permissions, error) {
	if !filepath.IsAbs(user.HomeDir) {
		logger.Warn(logSender, "user %v has invalid home dir: %v. Home dir must be an absolute path, login not allowed",
			user.Username, user.HomeDir)
		return nil, fmt.Errorf("Cannot login user with invalid home dir: %v", user.HomeDir)
	}
	if _, err := os.Stat(user.HomeDir); os.IsNotExist(err) {
		logger.Debug(logSender, "home directory \"%v\" for user %v does not exist, try to create", user.HomeDir, user.Username)
		err := os.MkdirAll(user.HomeDir, 0777)
		if err == nil {
			utils.SetPathPermissions(user.HomeDir, user.GetUID(), user.GetGID())
		}
	}

	if user.MaxSessions > 0 {
		activeSessions := getActiveSessions(user.Username)
		if activeSessions >= user.MaxSessions {
			logger.Debug(logSender, "authentication refused for user: %v, too many open sessions: %v/%v", user.Username,
				activeSessions, user.MaxSessions)
			return nil, fmt.Errorf("Too many open sessions: %v", activeSessions)
		}
	}

	json, err := json.Marshal(user)
	if err != nil {
		logger.Warn(logSender, "error serializing user info: %v, authentication rejected", err)
		return nil, err
	}
	p := &ssh.Permissions{}
	p.Extensions = make(map[string]string)
	p.Extensions["user"] = string(json)
	return p, nil
}

// If no host keys are defined we try to use or generate the default one.
func (c *Configuration) checkHostKeys(configDir string) error {
	var err error
	if len(c.Keys) == 0 {
		autoFile := filepath.Join(configDir, defaultPrivateKeyName)
		if _, err = os.Stat(autoFile); os.IsNotExist(err) {
			logger.Info(logSender, "No host keys configured and %s does not exist; creating new private key for server", autoFile)
			logger.InfoToConsole("No host keys configured and %s does not exist; creating new private key for server", autoFile)
			err = c.generatePrivateKey(autoFile)
		}

		c.Keys = append(c.Keys, Key{PrivateKey: defaultPrivateKeyName})
	}
	return err
}

func (c *Configuration) validatePublicKeyCredentials(conn ssh.ConnMetadata, pubKey string) (*ssh.Permissions, error) {
	var err error
	var user dataprovider.User

	p := c._checkBaseKey(conn.User(), pubKey)
	if p != nil {
		return p, nil
	}
	if user, err = dataprovider.CheckUserAndPubKey(dataProvider, conn.User(), pubKey); err == nil {
		//return loginUser(user, c)
		sp, err := loginUser(user, c)
		fmt.Printf("loginUser return %v,  err=%v\n", sp, err)
		return sp, err
	}
	return nil, err
}

func (c *Configuration) validatePasswordCredentials(conn ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
	var err error
	var user dataprovider.User

	if user, err = dataprovider.CheckUserAndPass(dataProvider, conn.User(), string(pass)); err == nil {
		return loginUser(user, c)
		//sp, err := loginUser(user, c)
		//return sp, err
	}
	return nil, err
}

// Generates a private key that will be used by the SFTP server.
func (c *Configuration) generatePrivateKey(file string) error {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	o, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer o.Close()

	pkey := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	if err := pem.Encode(o, pkey); err != nil {
		return err
	}

	return nil
}

func parseCommandPayload(command string) (string, []string, error) {
	parts := strings.Split(command, " ")
	if len(parts) < 2 {
		return parts[0], []string{}, nil
	}
	return parts[0], parts[1:], nil
}

