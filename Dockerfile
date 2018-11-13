FROM golang:1.11.1

COPY * ./

RUN go get
RUN go build
CMD ls -l