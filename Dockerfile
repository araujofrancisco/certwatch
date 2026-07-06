FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /certwatch ./cmd/certwatch/
RUN mkdir -p /data && chown 65534:65534 /data

FROM scratch

COPY --from=builder /certwatch /certwatch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /data /data
COPY config/default.yaml /config/default.yaml

EXPOSE 8080
ENTRYPOINT ["/certwatch"]
