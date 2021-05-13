FROM python:3.7-slim

RUN apt-get update && apt-get install -y wget zip
COPY ./nulog-inference-service/ /app/
COPY ./utils/nats_wrapper.py /app/nats_wrapper.py
COPY ./models/nulog/ /app/

RUN chmod a+rwx -R /app
WORKDIR /app

ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,utility

RUN pip install --no-cache-dir -r requirements.txt
RUN wget https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model.zip && unzip control-plane-model.zip

CMD [ "python", "start-nulog-inference.py" ]
