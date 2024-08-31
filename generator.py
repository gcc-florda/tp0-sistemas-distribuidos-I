import sys

def generate_docker_compose(file_name, num_clients):
    content = f"""name: tp0
services:
  server:
    container_name: server
    image: server:latest
    entrypoint: python3 /main.py
    environment:
      - PYTHONUNBUFFERED=1
      - LOGGING_LEVEL=DEBUG
    networks:
      - testing_net
"""
    
    for i in range(1, num_clients + 1):
        content += f"""
  client{i}:
    container_name: client{i}
    image: client:latest
    entrypoint: /client
    environment:
      - CLI_ID={i}
      - CLI_LOG_LEVEL=DEBUG
    networks:
      - testing_net
    depends_on:
      - server
"""

    content += """
networks:
  testing_net:
    ipam:
      driver: default
      config:
        - subnet: 172.25.125.0/24
"""

    with open(file_name, 'w') as file:
        file.write(content)

def main():
    if len(sys.argv) != 3:
        print("You must provide a file name and the number of clients")
        sys.exit(1)

    file_name = sys.argv[1]

    try:
        num_clients = int(sys.argv[2])
        if num_clients <= 0:
            raise ValueError("The number of clients must be a positive integer")
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)

    generate_docker_compose(file_name, num_clients)

    return

main()