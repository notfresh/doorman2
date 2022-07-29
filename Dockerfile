FROM golang:1.18
RUN mkdir /code
COPY . /code
WORKDIR /code
RUN go env -w GO111MODULE="on"
#RUN go get github.com/notfresh/doorman/go/cmd/doorman
RUN go build -o doorman ./go/cmd/doorman
RUN chmod a+x doorman
#RUN export PATH=$PATH:./bin
ENV PORT 6000
#CMD ./doorman -config=./config/config.yml -port=$PORT -debug_port=$(expr $PORT + 50) -etcd_endpoints=http://etcd:2379 -master_election_lock=/doorman.master -log_dir=/doorman_log_dir  -alsologtostderr
CMD ./endless.sh


