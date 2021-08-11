FROM python:3.8-slim as base

#Base builder
FROM base as builder1

RUN apt-get update \
    && apt-get install gcc -y \
    && apt-get clean

RUN python -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

COPY ./requirements.txt /requirements.txt

RUN pip install -r /requirements.txt

# Base torch
FROM builder1 as builder2

COPY ./requirements-torch.txt /requirements-torch.txt

# Work around for problem with docker copy - https://github.com/moby/moby/issues/21950
RUN find /opt/venv/ -type f > /files-to-delete.txt

RUN pip install -r /requirements-torch.txt

RUN cat /files-to-delete.txt | xargs  -d'\n' rm -f

# Final base image
FROM base as base-final

COPY --from=builder1 /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"


# Final torch image
FROM base-final as torch

COPY --from=builder2 /opt/venv /opt/venv

ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,utility

# Final elasticdump image
FROM base-final as elasticdump

RUN \
  apt-get update && \
  apt-get install -yqq wget gnupg && \
  echo "deb https://deb.nodesource.com/node_14.x buster main" > /etc/apt/sources.list.d/nodesource.list && \
  wget -qO- https://deb.nodesource.com/gpgkey/nodesource.gpg.key | apt-key add - && \
  echo "deb https://dl.yarnpkg.com/debian/ stable main" > /etc/apt/sources.list.d/yarn.list && \
  wget -qO- https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - && \
  apt-get update && \
  apt-get install -yqq nodejs yarn && \
  npm i -g npm@^6 && \
  npm i -g elasticdump && \
  apt-get purge -y --auto-remove wget && \
  rm -rf /var/lib/apt/lists/*