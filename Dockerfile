FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ytToDeemix .

FROM alpine:latest
RUN apk --no-cache add ca-certificates python3 py3-pip && \
    pip3 install --break-system-packages yt-dlp
COPY --from=builder /app/ytToDeemix /app/ytToDeemix
WORKDIR /app
EXPOSE 8080
CMD ["./ytToDeemix"]
