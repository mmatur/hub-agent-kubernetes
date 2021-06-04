FROM alpine

RUN apk --no-cache --no-progress add ca-certificates tzdata git \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

COPY hub-agent .

ENTRYPOINT ["/hub-agent"]
EXPOSE 80
EXPOSE 443
