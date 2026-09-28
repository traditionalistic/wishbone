# Server image: the static binary (built by push.sh) with the contract and the admin build. FROM scratch pulls
# nothing from Docker Hub, so a rebuild never hits its rate limit.
FROM scratch
COPY bin/wishbone-server /app/wishbone-server
COPY contract /app/contract
COPY admin /app/admin
# nobody: the server only listens and never writes to disk
USER 65534:65534
ENTRYPOINT ["/app/wishbone-server", "-addr", ":8080", "-contract", "/app/contract", "-admin", "/app/admin"]
