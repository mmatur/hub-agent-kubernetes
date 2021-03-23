FROM alpine

RUN apk --no-cache --no-progress add ca-certificates tzdata git \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

COPY neo-agent .

ENTRYPOINT ["/neo-agent"]
EXPOSE 80
EXPOSE 443
