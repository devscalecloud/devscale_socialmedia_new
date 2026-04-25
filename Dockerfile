FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/devscale-socialmedia .

FROM alpine:3.20

WORKDIR /app
RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates

COPY --from=builder /out/devscale-socialmedia /app/devscale-socialmedia
COPY --from=builder /src/templates /app/templates

ENV PORT=8080
EXPOSE 8080

USER app

CMD ["./devscale-socialmedia"]
