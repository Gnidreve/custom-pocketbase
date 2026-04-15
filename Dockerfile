FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/custom-pocketbase .

FROM alpine:3.23

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata nano curl

COPY --from=builder /out/custom-pocketbase /app/custom-pocketbase

RUN mkdir -p /app/pb_data /app/pb_public /app/pb_hooks

EXPOSE 8080

ENTRYPOINT ["/app/custom-pocketbase", "serve", "--http=0.0.0.0:8080", "--dir=/app/pb_data"]
