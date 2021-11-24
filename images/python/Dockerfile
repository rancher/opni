FROM registry.suse.com/suse/sle15:15.3 as base
RUN zypper -n ref \
    && zypper --non-interactive in python39 \
    && zypper --non-interactive in python39-pip \
    && zypper --non-interactive in python39-devel \
    && ln -s /usr/bin/python3.9 /usr/bin/python \
    && ln -s /usr/bin/pip3.9 /usr/bin/pip

#Base builder
FROM base as builder1

RUN zypper --non-interactive in gcc

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
