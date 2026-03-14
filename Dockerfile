FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /md2pdf .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /md2pdf /md2pdf
COPY web/ /web/
WORKDIR /
EXPOSE 5000
ENTRYPOINT ["/md2pdf"]
