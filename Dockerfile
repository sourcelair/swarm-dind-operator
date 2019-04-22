FROM golang:1.12 AS builder

WORKDIR /usr/src/app

COPY go.sum go.mod /usr/src/app/
RUN go mod download

COPY . /usr/src/app

RUN CGO_ENABLED=0 go build -a -installsuffix nocgo -o swarm-operator-dind


FROM alpine

RUN apk --update add ca-certificates
COPY --from=builder /usr/src/app/swarm-operator-dind /usr/bin/swarm-operator-dind
CMD ["/usr/bin/swarm-operator-dind"]
