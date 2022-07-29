#!/bin/bash
#set j=2
#while true
#do
#        let "j=j+1"
#        echo "----------j is $j--------------"
#        sleep 3s
#done
export PORT=6000
./doorman -config=./config/config.yml -port=$PORT -debug_port=$(expr $PORT + 50) -etcd_endpoints=http://etcd:2379 -master_election_lock=/doorman.master -log_dir=/doorman_log_dir  -alsologtostderr