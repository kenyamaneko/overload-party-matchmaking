FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /matchmaking ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /matchmaking /app/matchmaking
EXPOSE 9004
ENTRYPOINT ["/app/matchmaking"]
