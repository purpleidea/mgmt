FROM golang:1.24

LABEL maintainer="Michał Czeraszkiewicz <contact@czerasz.com>"

ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /go/src/mgmt

# Copy module files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree
COPY . /go/src/mgmt

# Install system and golang dependencies
RUN make deps

# Build the binary
RUN make build
