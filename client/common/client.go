package common

import (
	"bytes"
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

func (c *Client) OpenBetsFromFile() (*os.File, error) {
	file, err := os.Open(c.config.DataFilePath)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (c *Client) SendBatches() error {

	file, err := c.OpenBetsFromFile()

	if err != nil {
		return err
	}

	const maxBatchSize = 8 * 1024
	batchSize := c.config.BatchMaxAmount
	var currentBatch []string
	var currentBatchSize int

	reader := csv.NewReader(file)

	for {
		record, err := reader.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if len(record) != 5 {
			continue
		}

		bet := c.parseBet(record)

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

	file.Close()
	return nil
}

func (c *Client) FinishBets() error {
	finishMessage := fmt.Sprintf("%v|FINISHED\n", c.config.ID)

	_, err := c.sendMessage(finishMessage)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) RequestWinners() error {
	winnerMessage := fmt.Sprintf("%v|REQUEST_WINNERS\n", c.config.ID)

	msg, err := c.sendMessage(winnerMessage)
	if err != nil {
		return err
	}

	if strings.HasPrefix(msg, "WINNERS:") {
		winners := strings.TrimPrefix(msg, "WINNERS:")
		winners = strings.TrimSpace(winners)
		var winnerCount int
		if winners == "" {
			winnerCount = 0
		} else {
			winnersIds := strings.Split(winners, "|")
			winnerCount = len(winnersIds)
		}
		log.Infof("action: consulta_ganadores | result: success | cant_ganadores: %v", winnerCount)
	} else if strings.TrimSpace(msg) == "NOT_READY" {
		log.Infof("action: consulta_ganadores | result: not_ready | client_id: %v", c.config.ID)
	} else {
		log.Errorf("action: consulta_ganadores | result: fail | msg: %v", msg)
	}

	return nil
}

func (c *Client) StartClientLoop() {
	if err := c.createClientSocket(); err != nil {
		c.closeConnection("create_client_socket", err)
		return
	}

	go c.handleGracefulShutdown()

	err := c.SendBatches()
	if err != nil {
		c.closeConnection("send_batches", err)
		return
	}

	err = c.FinishBets()
	if err != nil {
		c.closeConnection("finish_bets", err)
		return
	}

	err = c.RequestWinners()
	if err != nil {
		c.closeConnection("request_winners", err)
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
	if err.Error() != "client already shutdown" && c.conn != nil {
		log.Infof("action: client_shutdown | result: success | client_id: %v", c.config.ID)
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

	if msg == "BATCH_RECEIVED" {
		log.Infof("action: batch_enviado | result: success")
	} else {
		log.Errorf("action: batch_enviado | result: fail")
	}

	time.Sleep(c.config.LoopPeriod)
	return nil
}

func (c *Client) sendMessage(message string) (string, error) {
	if c.conn == nil || !c.isRunning {
		log.Criticalf("action: connect | result: fail | client_id: %v | error: client already shutdown", c.config.ID)
		return "", errors.New("client already shutdown")
	}

	buffer := new(bytes.Buffer)

	err := binary.Write(buffer, binary.BigEndian, uint32(len(message)))
	if err != nil {
		log.Errorf("action: write_message_length | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	err = binary.Write(buffer, binary.BigEndian, []byte(message))
	if err != nil {
		log.Errorf("action: write_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	err = c.sendFullMessage(*buffer)

	if err != nil {
		log.Errorf("action: send_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	log.Infof("action: send_message | result: success | client_id: %v", c.config.ID)

	lengthBuffer := make([]byte, 4)
	_, err = io.ReadFull(c.conn, lengthBuffer)
	if err != nil {
		log.Errorf("action: receive_message_length | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	messageLength := binary.BigEndian.Uint32(lengthBuffer)

	messageBuffer := make([]byte, messageLength)
	_, err = io.ReadFull(c.conn, messageBuffer)
	if err != nil {
		log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return "", err
	}

	fullMessage := string(messageBuffer)

	log.Infof("action: receive_message | result: success | client_id: %v | msg: %v", c.config.ID, fullMessage)

	return fullMessage, nil
}

func (c *Client) sendFullMessage(buffer bytes.Buffer) error {
	fullMessageLength := buffer.Len()

	bytesSent := 0

	for bytesSent < fullMessageLength {
		n, err := c.conn.Write(buffer.Bytes())
		if err != nil {
			return err
		}
		bytesSent += n
	}

	return nil
}

func (c *Client) parseBet(record []string) string {
	return fmt.Sprintf("%v|%v|%v|%v|%v|%v\n",
		c.config.ID,
		record[0],
		record[1],
		record[2],
		record[3],
		record[4],
	)
}
