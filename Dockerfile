# Used by GoReleaser: the prebuilt krsec binary is copied in at release time.
# The image ships no shell; mount config and storage under /data.
# Note: set server.host to 0.0.0.0 in config.yaml — the default "localhost"
# is unreachable from outside the container.
FROM gcr.io/distroless/static-debian12:nonroot

COPY krsec /usr/local/bin/krsec

WORKDIR /data
VOLUME /data
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/krsec"]
CMD ["-config", "/data/config.yaml"]
