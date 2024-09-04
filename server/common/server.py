import socket
import logging
import signal
import struct
from common.utils import Bet, store_bets


class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self._running = True

        signal.signal(signal.SIGTERM, self._handle_sigterm)

    def _handle_sigterm(self, signum, frame):
        logging.info("action: server_graceful_shutdown | result: in_progress")
        self._running = False
        self._server_socket.close()
        logging.info("action: server_graceful_shutdown | result: success")

    def run(self):
        """
        Dummy Server loop

        Server that accept a new connections and establishes a
        communication with a client. After client with communucation
        finishes, servers starts to accept new connections again
        """

        while self._running:
            client_sock = self.__accept_new_connection()
            if not self._running:
                break
            self.__handle_client_connection(client_sock)

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket

        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            full_message = self.__receive_full_message(client_sock)
            
            if not full_message:
                return
            
            message = self.__process_message(full_message)

            self.__send_full_message(client_sock, "{}\n".format(message).encode('utf-8'))
            
        except OSError as e:
            logging.error("action: receive_message | result: fail | error: {e}")
        finally:
            client_sock.close()

    def __accept_new_connection(self):
        """
        Accept new connections

        Function blocks until a connection to a client is made.
        Then connection created is printed and returned
        """

        # Connection arrived
        logging.info('action: accept_connections | result: in_progress')

        try:
            c, addr = self._server_socket.accept()
        except:
            return
        
        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c

    def __receive_full_message(self, client_sock):
        data_length = client_sock.recv(4)
        if not data_length:
            return None
        message_length = struct.unpack('!I', data_length)[0]
        data = b''
        while len(data) < message_length:
            packet = client_sock.recv(message_length - len(data))
            if not packet:
                return None
            data += packet
        return data.decode('utf-8')
    
    def __send_full_message(self, sock, message):
        try:
            length_message = len(message)
            buffer = struct.pack('!I', length_message) + message
            sock.sendall(buffer)
        except socket.error as e:
            logging.error(f"action: send_message | result: fail | error: {e}")
    
    def __process_message(self, data):
        data = data.split('|')
        if len(data) == 6:
            bet = Bet(
                agency=data[0],
                first_name=data[1],
                last_name=data[2],
                document=data[3],
                birthdate=data[4],
                number=data[5].rstrip('\n')
            )
            try:
                store_bets([bet])
                logging.info(f'action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}')
                return data
            except Exception as e:
                logging.error(f'action: store_bets | result: fail | error: {e}')
        else:
            logging.error("action: process_message | result: fail | error: invalid_message_format")