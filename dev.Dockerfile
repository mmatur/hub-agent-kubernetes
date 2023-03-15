# syntax=docker/dockerfile:1.2
FROM alpine

RUN apk --no-cache --no-progress add ca-certificates tzdata git libc6-compat \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

COPY hub-agent-kubernetes .

EXPOSE 80
EXPOSE 443
EXPOSE 40000

ENTRYPOINT ["./hub-agent-kubernetes"]
