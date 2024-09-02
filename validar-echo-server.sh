#!/bin/bash
MESSAGE='Hello Server!'
PORT=12345
TIMEOUT=2

server_ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' server)

response=$(echo $MESSAGE | docker run -i --rm --network=tp0_testing_net --name=nc subfuzion/netcat -w $TIMEOUT $server_ip $PORT)

if [ "$response" = "$MESSAGE" ]; then
    echo "action: test_echo_server | result: success"
else
    echo "action: test_echo_server | result: fail"
fi