FROM golang:1-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/imitm .

FROM alpine:3

RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 imitm \
    && adduser -S -D -H -u 10001 -G imitm imitm \
    && mkdir -p /app/certs /app/data \
    && chown -R imitm:imitm /app

WORKDIR /app
COPY --from=build /out/imitm /usr/local/bin/imitm

USER imitm:imitm
EXPOSE 3129 3128
VOLUME ["/app/certs", "/app/data"]
ENTRYPOINT ["/usr/local/bin/imitm"]
