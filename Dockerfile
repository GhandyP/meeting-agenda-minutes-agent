# Build stage: the module is pinned by go.mod, so the toolchain follows it.
FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/meeting-agenda-minutes-agent .

# Runtime stage: a static binary with no shell, no package manager and no root.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/meeting-agenda-minutes-agent /usr/local/bin/meeting-agenda-minutes-agent
WORKDIR /home/nonroot
EXPOSE 8080

# Inside a container the app must bind the container interface, so the image
# states -allow-remote explicitly. Publish it to loopback only:
#   docker run --rm -p 127.0.0.1:8080:8080 -e GOOGLE_API_KEY=... meeting-agenda-minutes-agent:local
# The server has no authentication and writes the dry-run artifacts it is asked
# for, so never publish it on a public interface.
ENTRYPOINT ["/usr/local/bin/meeting-agenda-minutes-agent"]
CMD ["serve", "-addr", "0.0.0.0:8080", "-out", "/home/nonroot/out", "-allow-remote"]
