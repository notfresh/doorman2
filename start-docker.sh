#docker run -d --name etcd --rm \
#    --network zx-net \
#    --publish 2379:2379 \
#    --publish 2380:2380 \
#    --env ALLOW_NONE_AUTHENTICATION=yes \
#    --env ETCD_ADVERTISE_CLIENT_URLS=http://etcd-server:2379 \
#    bitnami/etcd:latest


# 指定V2
docker run -d --name etcd --rm \
    --network zx-net \
    --publish 2379:2379 \
    --publish 2380:2380 \
    --env ALLOW_NONE_AUTHENTICATION=yes \
    --env ETCD_ADVERTISE_CLIENT_URLS=http://etcd-server:2379 \
    bitnami/etcd:latest etcd --enable-v2=true

# 6000是TCP端口，而6050才是http接口
docker run  --name doorman --rm --network zx-net \
--publish 6000:6000 --publish 6050:6050 -it doorman:0.2
./bin/doorman -config=./config/config.yml -port=6000  -debug_port=6050 \
-etcd_endpoints=http://etcd-server:2379 -master_election_lock="/doorman.master" \
-log_dir=/doorman_log_dir  -alsologtostderr


# 测试
#docker run  --name doorman --rm --network zx-net \
#--publish 6000:6000 --publish 6050:6050 -it doorman:0.2 bash