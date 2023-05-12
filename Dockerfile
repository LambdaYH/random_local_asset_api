FROM golang:1.20 AS builder
WORKDIR /build
COPY . /build
RUN go mod download
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -tags=jsoniter -o app .

FROM istio/distroless
COPY --from=builder ["/build/app", "/"]"
ENV GIN_MODE=release
ENTRYPOINT ["/app"]