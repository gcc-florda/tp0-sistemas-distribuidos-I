import socket
import logging
import signal
import struct
import multiprocessing
from common.utils import AGENCIES, FINISH_MESSAGE_HEADER, WINNERS_MESSAGE_HEADER, Bet, store_bets, load_bets, has_won

class Server:
    def __init__(self, port, listen_backlog):
        # Initialize server socket
        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.bind(('', port))
        self._server_socket.listen(listen_backlog)
        
        self.barrier = multiprocessing.Barrier(AGENCIES)
        manager = multiprocessing.Manager()
        self._running = manager.Value('b', True)
        self.finished_agencies = manager.list()
        self.bets = manager.list()
        self.file_lock = manager.Lock()
        self.bets_lock = manager.Lock()
        self.processes = []

        signal.signal(signal.SIGTERM, self.__handle_sigterm)

    def __handle_sigterm(self, signum, frame):
        """
        Handle SIGTERM signal to gracefully shut down the server.
        Closes the server socket and terminates all active processes.
        """
        logging.info("action: server_graceful_shutdown | result: in_progress")
        self._running.value = False

        self._server_socket.close()

        for process in self.processes:
            if process.is_alive():
                try:
                    process.terminate() # kill the process
                    process.join() # wait it to end
                    logging.info(f"action: process_terminated | process_id: {process.pid} | result: success")
                except Exception as e:
                    logging.warn(f"action: process_terminated | process_id: {process.pid} | result: fail | error: {e}")

        logging.info("action: server_graceful_shutdown | result: success")

    def run(self):
        """
        Dummy Server loop

        Server that accept a new connections and establishes a
        communication with a client. After client with communucation
        finishes, servers starts to accept new connections again
        """        
        while self._running.value:
            client_sock = self.__accept_new_connection()
            if not self._running.value:
                break
            process = multiprocessing.Process(target=self.__handle_client_connection, args=(client_sock,))
            self.processes.append(process)
            process.start()

        for process in self.processes:
            process.join()

    def __handle_client_connection(self, client_sock):
        """
        Read message from a specific client socket and closes the socket

        If a problem arises in the communication with the client, the
        client socket will also be closed
        """
        try:
            while self._running.value:
                full_message = self.__receive_full_message(client_sock)

                if not full_message or not self._running.value:
                    break

                messages = full_message.split('\n')
                
                if len(messages) == 2:
                    message = messages[0].split('|')
                    if message[1] == FINISH_MESSAGE_HEADER:
                        if not self.__process_finish_message(client_sock, message):
                            break
                    elif message[1] == WINNERS_MESSAGE_HEADER:
                        self.__process_winners_message(client_sock, message)
                        break
                elif not self.__process_bet_message(client_sock, messages): break
        except OSError as e:
            logging.error("action: receive_message | result: fail | error: {e}")
        finally:
            client_sock.close()
            logging.info("action: client_connection | result: closed")

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

    def __receive_full_message(self, client_sock):
        """
        Receive a full message from the client socket.
        The first 4 bytes of the message represent the length of the message.
        """
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
        """
        Send a full message to the client socket.
        The first 4 bytes of the message represent the length of the message to be sent.
        """
        try:
            length_message = len(message)
            buffer = struct.pack('!I', length_message) + message
            sock.sendall(buffer)
        except socket.error as e:
            logging.error(f"action: send_message | result: fail | error: {e}")
    
    def __get_acency_winners(self, agency_id):
        """
        Retrieve the list of winners DNIs for a specific agency.
        """
        winners = []
        self.bets_lock.acquire()
        for bet in self.bets:
            if bet.agency == int(agency_id) and has_won(bet):
                winners.append(bet.document)
        self.bets_lock.release()
        return winners
    
    def __process_finish_message(self, client_sock, message):
        """
        Process a 'FINISHED' message from an agency and then send a response.
        """
        if len(self.finished_agencies) == AGENCIES - 1:
            self.bets[:] = list(load_bets())
            logging.info("action: sorteo | result: success")
        self.finished_agencies.append(message[0])
        
        if self._running.value:
            try:
                self.barrier.wait()
            except Exception as e:
                logging.warn(f"action: barrier_wait | result: fail | error: server already shutdown")
                return False
        
        self.__send_full_message(client_sock, "FINISHED RECEIVE\n".encode('utf-8'))
        return True
    
    def __process_winners_message(self, client_sock, message):
        """
        Process a 'REQUEST_WINNERS' message from an agency and then send a response.
        """
        winners_list = self.__get_acency_winners(message[0])
        winners = '|'.join(winners_list)
        self.__send_full_message(client_sock, f"WINNERS:{winners}\n".encode('utf-8'))

    def __process_bet_message(self, client_sock, messages):
        """
        Process a batch of bet messages from an agency and then send a response.
        """
        success_count = 0
        fail_count = 0

        for message in messages:
            if (not self._running.value):
                return False
            if len(message) != 0:
                if self.__process_bet(message):
                    success_count += 1
                else:
                    fail_count += 1

        if not self._running.value:
            return False

        if fail_count == 0:
            logging.info(f'action: apuesta_recibida | result: success | cantidad: {success_count}')
            self.__send_full_message(client_sock, "BATCH_RECEIVED\n".encode('utf-8'))
        else:
            logging.error(f'action: apuesta_recibida | result: fail | cantidad: {success_count}')
            logging.warn(f'action: apuesta_rechazada | result: fail | cantidad: {fail_count}')
            self.__send_full_message(client_sock, "BATCH_FAILED\n".encode('utf-8'))
        return True
    
    def __process_bet(self, data):
        """
        Process a single bet.
        """
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
            self.file_lock.acquire()
            store_bets([bet])
            self.file_lock.release()
            logging.info(f'action: apuesta_almacenada | result: success | dni: {bet.document} | numero: {bet.number}')
            return True
        except Exception as e:
            logging.error(f'action: store_bets | result: fail | error: {e}')
            return False