package common

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/op/go-logging"
	"encoding/csv"
)

var log = logging.MustGetLogger("log")

// ClientConfig Configuration used by the client
type ClientConfig struct {
	ID            string
	ServerAddress string
	LoopAmount    int
	LoopPeriod    time.Duration
	BatchMaxAmount int
	DataFilePath  string
}

// Client Entity that encapsulates how
type Client struct {
	config ClientConfig
	conn   net.Conn
	shutdown chan os.Signal
	isRunning bool
}

// NewClient Initializes a new client receiving the configuration
// as a parameter
func NewClient(config ClientConfig) *Client {
	client := &Client{
		config: config,
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
	batchSize := c.config.BatchMaxAmount
	totalBets := len(bets)
	for i := 0; i < totalBets; i += batchSize {
		end := i + batchSize

		if end > totalBets {
			end = totalBets
		}

		batch := bets[i:end]

		batch_message := strings.Join(batch, "")

		length_message := uint32(len(batch_message))

		c.createClientSocket()

		if c.conn == nil || !c.isRunning {
			log.Criticalf("action: connect | result: fail | client_id: %v | error: server closed", c.config.ID)
			return nil
		}

		err := binary.Write(c.conn, binary.BigEndian, length_message)

		if err != nil {
			log.Errorf("action: send_message_length | result: fail | client_id: %v | error: %v", c.config.ID, err)
			c.conn.Close()
			return err
		}

		log.Infof("action: send_message_length | result: success | client_id: %v", c.config.ID)

		fmt.Fprintf(c.conn, batch_message)

		msg, err := bufio.NewReader(c.conn).ReadString('\n')
		c.conn.Close()

		if err != nil {
			log.Errorf("action: receive_message | result: fail | client_id: %v | error: %v",
				c.config.ID,
				err,
			)
			return err
		}

		log.Infof("action: receive_message | result: success | client_id: %v | msg: %v",
			c.config.ID,
			msg,
		)

		if msg == "OK\n" {
			log.Infof("action: batch_enviado | result: success")
		} else {
			log.Errorf("action: batch_enviado | result: fail")
		}

		time.Sleep(c.config.LoopPeriod)
	}
	
	return nil
}

func (c *Client) StartClientLoop() {
	go c.handleGracefulShutdown()

	bets, err := c.ReadBetsFromFile()
	if err != nil {
		log.Criticalf("action: read_bets | result: fail | client_id: %v | error: %v", c.config.ID, err)
		return
	}

	err = c.SendBatches(bets)
	
	if err != nil {
		log.Errorf("action: send_batches | result: fail | client_id: %v | error: %v", c.config.ID, err)
		c.conn.Close()
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
