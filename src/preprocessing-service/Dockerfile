FROM python:3.8-slim
WORKDIR /code
COPY ./preprocessing-service/preprocess.py .
COPY ./preprocessing-service/masker.py .
COPY ./utils/nats_wrapper.py .
COPY ./preprocessing-service/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
CMD [ "python", "./preprocess.py" ]
