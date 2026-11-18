# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cinder ./cmd/cinder

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cinder /cinder
USER nonroot:nonroot
EXPOSE 8080
# No shell/curl in distroless: the binary probes its own /healthz.
HEALTHCHECK --interval=10s --timeout=4s --start-period=5s --retries=3 CMD ["/cinder", "healthcheck"]
ENTRYPOINT ["/cinder"]
