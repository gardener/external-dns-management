#!/bin/sh

docker run --privileged -it --rm -v /var/run/docker.sock:/var/run/docker.sock  dind-go bash
