FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY . /build
RUN go mod download
RUN go build -tags=jsoniter -o api_server .

FROM alpine:latest
COPY --from=builder /build/api_server /
RUN mkdir -p /assets
ENV GIN_MODE=release
CMD /api_server
