# Upgrading to go 1.16+ breaks this image for now. Maybe see why someday.
FROM golang:1.15

RUN go get k8s.io/code-generator; exit 0
RUN go get k8s.io/apimachinery; exit 0

ARG USER=$USER
ARG UID=1000
ARG GID=1000
RUN useradd -m ${USER} --uid=${UID} && echo "${USER}:" chpasswd

USER ${UID}:${GID}
WORKDIR $GOPATH/src/k8s.io/code-generator
