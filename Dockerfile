FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN --mount=type=secret,id=GO_MODULES_TOKEN \
    git config --global url."https://x-access-token:$(cat /run/secrets/GO_MODULES_TOKEN)@github.com/kenyamaneko/".insteadOf "https://github.com/kenyamaneko/" && \
    GOPRIVATE=github.com/kenyamaneko/* go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /matchmaking ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /matchmaking /app/matchmaking
EXPOSE 9004
ENTRYPOINT ["/app/matchmaking"]
