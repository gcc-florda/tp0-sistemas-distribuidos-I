#!/bin/bash
MESSAGE='Hello Server!'
PORT=12345
docker_network=$(docker network inspect tp0_testing_net)
server_ip=$(echo "$docker_network" | grep -A 10 '"Name": "server"' | grep '"IPv4Address"' | awk -F'"' '{print $4}' | cut -d'/' -f1 | head -n 1)

if [ -z "$server_ip" ]; then
    echo "action: test_echo_server | result: fail"
    exit 1
fi

response=$(echo $MESSAGE | docker run --rm --network=tp0_testing_net -i subfuzion/netcat $server_ip $PORT)

if [ "$response" == "$MESSAGE" ]; then
    echo "action: test_echo_server | result: success"
else
    echo "action: test_echo_server | result: fail"
fi