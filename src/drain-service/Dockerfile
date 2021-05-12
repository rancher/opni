FROM python:3.8-slim
WORKDIR /code
COPY ./drain-service/drain_training_inferencing.py .
COPY ./drain-service/drain3.ini .
COPY ./drain-service/requirements.txt .
ADD ./drain-service/drain3 /code/drain3
COPY ./utils/nats_wrapper.py .
RUN pip install --no-cache-dir -r requirements.txt
CMD ["python", "./drain_training_inferencing.py"]