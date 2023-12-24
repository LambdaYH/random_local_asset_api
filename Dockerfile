FROM golang:1.21 AS builder
WORKDIR /build
COPY . /build
RUN go mod download
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -tags=jsoniter -o api_server .

FROM istio/distroless
COPY --from=builder ["/build/api_server", "/"]"
ENV GIN_MODE=release
ENTRYPOINT ["/api_server"]