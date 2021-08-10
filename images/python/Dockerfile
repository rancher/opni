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

RUN cat /files-to-delete.txt | xargs rm -f

# Final base image
FROM base as base-final

COPY --from=builder1 /opt/venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"


# Final torch image
FROM base-final as torch

COPY --from=builder2 /opt/venv /opt/venv
