
./bin/doorman  -config=./config/config.yml \
-etcd_endpoints=http://localhost:2379 -master_election_lock=/doorman.master \
 -log_dir="./doorman_log_dir"