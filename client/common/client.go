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

	"encoding/csv"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID             string
	ServerAddress  string
	LoopAmount     int
	LoopPeriod     time.Duration
	BatchMaxAmount int
	DataFilePath   string
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

func (c *Client) ReadBetsFromFile() ([]string, error) {
	file, err := os.Open(c.config.DataFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var bets []string
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	for _, record := range records {
		if len(record) < 5 {
			continue
		}
		bets = append(bets, fmt.Sprintf("%v|%v|%v|%v|%v|%v\n",
			c.config.ID,
			record[0],
			record[1],
			record[2],
			record[3],
			record[4],
		))
	}
	return bets, nil
}

func (c *Client) SendBatches(bets []string) error {
	const maxBatchSize = 8 * 1024
	batchSize := c.config.BatchMaxAmount
	var currentBatch []string
	var currentBatchSize int

	for _, bet := range bets {
		betSize := len(bet)

		if currentBatchSize+betSize > maxBatchSize || len(currentBatch) >= batchSize {
			if err := c.sendBatch(currentBatch); err != nil {
				return err
			}

			currentBatch = []string{}
			currentBatchSize = 0
		}

		currentBatch = append(currentBatch, bet)
		currentBatchSize += betSize
	}

	if len(currentBatch) > 0 {
		if err := c.sendBatch(currentBatch); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) StartClientLoop() {
	go c.handleGracefulShutdown()

	bets, err := c.ReadBetsFromFile()
	if err != nil {
		c.closeConnection("read_bets", err)
		return
	}

	err = c.SendBatches(bets)
	if err != nil {
		c.closeConnection("send_batches", err)
		return
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
	if err.Error() != "client already shutdown" {
		c.conn.Close()
	}
}

func (c *Client) sendBatch(batch []string) error {
	batchMessage := strings.Join(batch, "")

	msg, err := c.sendMessage(batchMessage)
	if err != nil {
		return err
	}

	msg = strings.TrimSpace(msg)

	if msg == "OK" {
		log.Infof("action: batch_enviado | result: success")
	} else {
		log.Errorf("action: batch_enviado | result: fail")
	}

	time.Sleep(c.config.LoopPeriod)
	return nil
}

func (c *Client) sendMessage(message string) (string, error) {
	lengthMessage := uint32(len(message))

	c.createClientSocket()

	if c.conn == nil || !c.isRunning {
		log.Criticalf("action: connect | result: fail | client_id: %v | error: client already shutdown", c.config.ID)
		return "", errors.New("client already shutdown")
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

	if err != nil {
		log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	fullMessage := string(readMessage)

	c.conn.Close()

	log.Infof("action: receive_message | result: success | client_id: %v | msg: %v", c.config.ID, fullMessage)

	return fullMessage, nil
}
