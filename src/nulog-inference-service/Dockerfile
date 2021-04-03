FROM python:3.7-slim
COPY ./nulog-inference-service/ /app/
COPY ./utils/nats_wrapper.py /app/nats_wrapper.py

RUN chmod a+rwx -R /app
WORKDIR /app

ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,utility

RUN pip install --no-cache-dir -r requirements.txt
CMD [ "python", "start-nulog-inference.py" ]
