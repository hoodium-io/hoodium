# syntax=docker/dockerfile:1.7-labs

#
# Build layer
#
# NOTE: must match the `go` directive in go.mod (1.27). Do NOT use a
# `-bullseye` tag here: the golang image publishes no 1.27 bullseye variant
# (`golang:1.27-bullseye` and `golang:1.27.3-bullseye` are both 404), and
# bookworm is the better match for the distroless debian12 runtime below.
FROM golang:1.27.3-bookworm AS build

WORKDIR /go/src/github.com/hoodium-io/hoodium

RUN apt-get update -y && \
    apt-get install jq -y

RUN curl -sL https://deb.nodesource.com/setup_18.x | bash && \
    apt-get update -y && \
    apt-get install -y nodejs

COPY .git .git
COPY go.mod go.sum ./
RUN go mod download

COPY --parents ./**/Makefile ./
COPY --parents ./**/*.txt ./
COPY --parents ./**/.keep ./
COPY --parents ./**/*.json ./
COPY --parents ./**/*.go ./

RUN make build

#
# Busybox layer as source of shell commands
#
FROM busybox:1.36.0-glibc AS busybox

#
# Production layer
#
FROM gcr.io/distroless/base-nossl-debian12:nonroot AS production

ADD --chmod=755 https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64 /bin/jq

COPY --from=busybox /bin/sh /bin/cat /bin/test /bin/ls /bin/grep /bin/awk /bin/tail /bin/rm /bin/yes /bin/
COPY --from=build /go/src/github.com/hoodium-io/hoodium/build/runed /usr/bin/runed
COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]

CMD ["runed"]
