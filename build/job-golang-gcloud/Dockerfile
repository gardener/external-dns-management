# Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.12.4

# used https://github.wdf.sap.corp/kubernetes/job-image-golang/blob/master/Dockerfile as template

# install SAP root CA.
RUN wget http://aia.pki.co.sap.com/aia/SAP%20Global%20Root%20CA.crt -P /usr/local/share/ca-certificates/ \
    && update-ca-certificates

# set env-variable so that 'requests' python library uses the system's trust-store
ENV REQUESTS_CA_BUNDLE /etc/ssl/certs/ca-certificates.crt

RUN mkdir -p /cc/
RUN git clone https://github.com/gardener/cc-utils /cc/utils

RUN echo "deb http://ftp.de.debian.org/debian testing main" >> /etc/apt/sources.list && \
    echo "tango-common tango-common/tango-host string tango-host:20000" | debconf-set-selections && \
    apt-get update -y && \
    apt-get -t testing install python3.6 -y && \
    apt-get -t testing install python3-pip -y && \
    pip3 install -r /cc/utils/requirements.txt

# add cc-utils' cli.py to PATH and PYTHONPATH
ENV PATH /cc/utils/:/cc/utils/bin:$PATH
ENV PYTHONPATH /cc/utils

# install gcloud sdk & kubectl
RUN curl -sSL https://sdk.cloud.google.com | bash
ENV PATH $PATH:/root/google-cloud-sdk/bin
RUN gcloud components install --quiet kubectl