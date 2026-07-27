# Builds the three binaries and nothing else. No credentials are baked in, and
# none are needed: every data source used here is public, and the only broker
# implemented is the paper one.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/signalctl ./cmd/signalctl && \
    CGO_ENABLED=0 go build -trimpath -o /out/signald   ./cmd/signald && \
    CGO_ENABLED=0 go build -trimpath -o /out/bookd     ./cmd/bookd && \
    CGO_ENABLED=0 go build -trimpath -o /out/fetchdata ./cmd/fetchdata

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 signals
COPY --from=build /out/ /usr/local/bin/
COPY --from=build /src/config.example.json /etc/marketsignals/config.example.json

# Unprivileged: this process reads public prices and writes a file. Nothing it
# does needs root, and a container that trades should be the least privileged
# thing on the box rather than the most.
USER signals
WORKDIR /home/signals
VOLUME ["/home/signals/data"]

ENTRYPOINT ["signalctl"]
CMD ["--help"]
