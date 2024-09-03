package common

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
}

// Client Entity that encapsulates how
type Client struct {
	config    ClientConfig
	conn      net.Conn
	shutdown  chan os.Signal
	isRunning bool
}

// NewClient Initializes a new client receiving the configuration
// as a parameter
func NewClient(config ClientConfig) *Client {
	client := &Client{
		config:    config,
		shutdown:  make(chan os.Signal, 1),
		isRunning: true,
	}
	signal.Notify(client.shutdown, syscall.SIGTERM)
	return client
}

// CreateClientSocket Initializes client socket. In case of
// failure, error is printed in stdout/stderr and exit 1
// is returned
func (c *Client) createClientSocket() error {
	conn, err := net.Dial("tcp", c.config.ServerAddress)
	if err != nil {
		log.Criticalf(
			"action: connect | result: fail | client_id: %v | error: %v",
			c.config.ID,
			err,
		)
	}
	c.conn = conn
	return nil
}

// StartClientLoop Send messages to the client until some time threshold is met
func (c *Client) StartClientLoop() {
	go c.handleGracefulShutdown()

	message := fmt.Sprintf("%v|%v|%v|%v|%v|%v\n",
		c.config.ID,
		os.Getenv("NOMBRE"),
		os.Getenv("APELLIDO"),
		os.Getenv("DOCUMENTO"),
		os.Getenv("NACIMIENTO"),
		os.Getenv("NUMERO"))

	msg, err := c.sendMessage(message)

	if err != nil {
		c.closeConnection("receive_message", err)
		return
	}

	msg = strings.TrimSpace(msg)
	message = strings.TrimSpace(message)

	if msg == message {
		log.Infof("action: apuesta_enviada | result: success | dni: %s | numero: %s", os.Getenv("DOCUMENTO"), os.Getenv("NUMERO"))
	} else {
		log.Errorf("action: apuesta_enviada | result: fail | dni: %s | numero: %s", os.Getenv("DOCUMENTO"), os.Getenv("NUMERO"))
	}

	log.Infof("action: loop_finished | result: success | client_id: %v", c.config.ID)
}

func (c *Client) handleGracefulShutdown() {
	<-c.shutdown
	log.Infof("action: client_graceful_shutdown | result: in_progress | client_id: %v", c.config.ID)
	c.isRunning = false
	if c.conn != nil {
		c.conn.Close()
	}
	log.Infof("action: client_graceful_shutdown | result: success | client_id: %v", c.config.ID)
}

func (c *Client) closeConnection(action string, err error) {
	log.Errorf("action: %s | result: fail | client_id: %v | error: %v", action, c.config.ID, err)
	if err.Error() != "server closed" {
		c.conn.Close()
	}
}

func (c *Client) sendMessage(message string) (string, error) {
	lengthMessage := uint32(len(message))

	c.createClientSocket()

	if c.conn == nil || !c.isRunning {
		log.Criticalf("action: connect | result: fail | client_id: %v | error: server closed", c.config.ID)
		return "", errors.New("server closed")
	}

	err := binary.Write(c.conn, binary.BigEndian, lengthMessage)
	if err != nil {
		log.Errorf("action: send_message_length | result: fail | client_id: %v | error: %v", c.config.ID, err)
		c.conn.Close()
		return "", err
	}

	log.Infof("action: send_message_length | result: success | client_id: %v", c.config.ID)

	fmt.Fprintf(c.conn, message)

	var length uint32

	err = binary.Read(c.conn, binary.BigEndian, &length)
	if err != nil {
		log.Infof("action: receive_message_length | result: fail | error: %v", err)
		return "", err
	}

	log.Infof("action: receive_message_length | result: success | message_length: %d", length)

	readMessage := make([]byte, length)
	_, err = io.ReadFull(c.conn, readMessage)
	c.conn.Close()

	fullMessage := string(readMessage)

	if err != nil {
		log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	log.Infof("action: receive_message | result: success | client_id: %v | msg: %v", c.config.ID, fullMessage)

	return fullMessage, nil
}
