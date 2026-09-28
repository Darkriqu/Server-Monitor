FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server-monitor ./cmd/server-monitor

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /var/lib/server-monitor
COPY --from=build /out/server-monitor /server-monitor
EXPOSE 9090
ENTRYPOINT ["/server-monitor"]
CMD ["--config", "/etc/server-monitor/config.toml"]
