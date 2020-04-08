FROM golang:1.14-alpine AS build-stage

WORKDIR /go/src/github.com/derfetzer/longhorn-monitor/
COPY . .

WORKDIR /go/src/github.com/derfetzer/longhorn-monitor/webhook

RUN go get -d -v ./...
RUN GOBIN=/bin go install -v ./...

WORKDIR /go/src/github.com/derfetzer/longhorn-monitor/healthcheck

RUN go get -d -v ./...
RUN GOBIN=/bin go install -v ./...

WORKDIR /go/src/github.com/derfetzer/longhorn-monitor/monitor

RUN go get -d -v ./...
RUN GOBIN=/bin go install -v ./...

# Final image.
FROM alpine:3.11
RUN apk --no-cache add \
  ca-certificates
COPY --from=build-stage /bin/webhook /usr/local/bin/longhorn-monitor/webhook
COPY --from=build-stage /bin/healthcheck /usr/local/bin/longhorn-monitor/healthcheck
COPY --from=build-stage /bin/monitor /usr/local/bin/longhorn-monitor/monitor
ENTRYPOINT ["/usr/local/bin/longhorn-monitor/webhook"]