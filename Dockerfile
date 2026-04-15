FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/custom-pocketbase .

FROM debian:bookworm-slim

WORKDIR /app

RUN useradd --system --uid 10001 pocketbase

COPY --from=builder /out/custom-pocketbase /app/custom-pocketbase

RUN mkdir -p /app/pb_data /app/pb_public /app/secrets && \
    chown -R pocketbase:pocketbase /app

USER pocketbase

EXPOSE 8090

CMD ["/app/custom-pocketbase", "serve", "--http=0.0.0.0:8090"]
