FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /drive-topology-exporter .

# Static binary; sas2ircu + /dev + /sys are provided by the host at runtime
# (run privileged with the relevant bind mounts — see README).
FROM gcr.io/distroless/static-debian12
COPY --from=build /drive-topology-exporter /drive-topology-exporter
EXPOSE 9101
ENTRYPOINT ["/drive-topology-exporter"]
