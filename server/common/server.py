import socket
import logging
import signal
import struct
import multiprocessing
from common.utils import Bet, store_bets, load_bets, has_won

AGENCIES = 5

class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        self._running = True
        
        manager = multiprocessing.Manager()
        self.finished_agencies = manager.list()
        self.bets = manager.list()
        self.file_lock = manager.Lock()

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
            process = multiprocessing.Process(target=self.__handle_client_connection, args=(client_sock,))
            process.start()
            process.join()

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket

        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            data_length = client_sock.recv(4)
            if not data_length:
                return

            message_length = struct.unpack('!I', data_length)[0]

            full_message = self.__receive_full_message(client_sock, message_length)

            if not full_message:
                return

            messages = full_message.decode('utf-8').split('\n')
            
            if len(messages) == 2:
                message = messages[0].split('|')
                if message[1] == 'FINISHED':
                    self.finished_agencies.append(message[0])

                    if len(self.finished_agencies) == AGENCIES:
                        logging.info("action: sorteo | result: success")
                        self.bets[:] = list(load_bets())
                    
                    self.__send_full_message(client_sock, "FINISHED RECEIVE\n".encode('utf-8'))

                elif message[1] == 'REQUEST_WINNERS':
                    if len(self.finished_agencies) < AGENCIES:
                        self.__send_full_message(client_sock, "NOT_READY\n".encode('utf-8'))
                    else:
                        winners_list = self.__get_acency_winners(message[0])
                        winners = '|'.join(winners_list)
                        self.__send_full_message(client_sock, f"WINNERS:{winners}\n".encode('utf-8'))

            else:
                success_count = 0
                fail_count = 0

                for message in messages:
                    if len(message) != 0:
                        if self.__process_message(message):
                            success_count += 1
                        else:
                            fail_count += 1

                if fail_count == 0:
                    logging.info(f'action: apuesta_recibida | result: success | cantidad: {success_count}')
                    self.__send_full_message(client_sock, "OK\n".encode('utf-8'))
                else:
                    logging.error(f'action: apuesta_recibida | result: fail | cantidad: {success_count}')
                    logging.warn(f'action: apuesta_rechazada | result: fail | cantidad: {fail_count}')
                    self.__send_full_message(client_sock, "FAIL\n".encode('utf-8'))

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

        logging.info('action: accept_connections | result: in_progress')

        try:
            c, addr = self._server_socket.accept()
        except:
            return

        logging.info(f'action: accept_connections | result: success | ip: {addr[0]}')
        return c

    def __receive_full_message(self, client_sock, length):
        data = b''
        while len(data) < length:
            packet = client_sock.recv(length - len(data))
            if not packet:
                return None
            data += packet
        return data
    
    def __send_full_message(self, sock, message):
        total_sent = 0
        length_message = len(message)
        
        try:
            sock.sendall(struct.pack('!I', length_message))
        except socket.error as e:
            logging.error(f"action: send_message_length | result: fail")
            return

        while total_sent < length_message:
            sent = sock.send(message[total_sent:])
            if sent == 0:
                logging.error(f'action: send_message | result: fail')
                break
            total_sent += sent
    
    def __process_message(self, data):
        bet_data = data.split('|')
        if len(bet_data) != 6:
            logging.error("action: process_message | result: fail | error: invalid_message_format")
            return False
        try:
            bet = Bet(
                agency=bet_data[0],
                first_name=bet_data[1],
                last_name=bet_data[2],
                document=bet_data[3],
                birthdate=bet_data[4],
                number=bet_data[5].rstrip('\n')
            )
        except Exception as e:
            logging.error(f'action: process_message | result: fail | error: {e}')
            return False
        try:
            with self.file_lock:
                store_bets([bet])
            logging.info(f'action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}')
            return True
        except Exception as e:
            logging.error(f'action: store_bets | result: fail | error: {e}')
            return False
    
    def __get_acency_winners(self, agency_id):
        winners = []
        for bet in self.bets:
            if bet.agency == int(agency_id) and has_won(bet):
                winners.append(bet.document)
        return winners