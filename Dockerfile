FROM golang:1.20-alpine as buildbase

WORKDIR /go/src/github.com/rarimo/relayer-svc
RUN apk add build-base
COPY vendor .
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /usr/local/bin/relayer-svc github.com/rarimo/relayer-svc

###
FROM alpine:3.9
COPY --from=buildbase /usr/local/bin/relayer-svc /usr/local/bin/relayer-svc

ENTRYPOINT ["relayer-svc"]
