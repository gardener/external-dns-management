#!/bin/sh

docker run -it --rm -v /var/run/docker.sock:/var/run/docker.sock dind-go bash
