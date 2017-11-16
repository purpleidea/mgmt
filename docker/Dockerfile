FROM golang:1.8

MAINTAINER Michał Czeraszkiewicz <contact@czerasz.com>

# Set the reset cache variable
# Read more here: http://czerasz.com/2014/11/13/docker-tip-and-tricks/#use-refreshedat-variable-for-better-cache-control
ENV REFRESHED_AT 2017-11-16

# Update the package list to be able to use required packages
RUN apt-get update

# Change the working directory
WORKDIR /go/src/mgmt

# Copy all the files to the working directory
COPY . /go/src/mgmt

# Install dependencies
RUN make deps

# Build the binary
RUN make build
